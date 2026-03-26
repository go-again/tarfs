package tarfs_test

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/klauspost/compress/zstd"
	lz4 "github.com/pierrec/lz4/v4"

	"github.com/go-again/tarfs"
)

// ---------------------------------------------------------------------------
// Archive builders
// ---------------------------------------------------------------------------

// buildTar creates a raw (uncompressed) tar archive from a file map.
// A nil value emits an explicit directory header; a non-nil value emits a file.
func buildTar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, data := range files {
		if data == nil {
			_ = tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(data)), Mode: 0o644})
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	return buf.Bytes()
}

func buildGzip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, data := range files {
		if data == nil {
			_ = tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(data)), Mode: 0o644})
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func buildZstd(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	tw := tar.NewWriter(enc)
	for name, data := range files {
		if data == nil {
			_ = tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(data)), Mode: 0o644})
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = enc.Close()
	return buf.Bytes()
}

func buildLz4(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	lw := lz4.NewWriter(&buf)
	tw := tar.NewWriter(lw)
	for name, data := range files {
		if data == nil {
			_ = tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(data)), Mode: 0o644})
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = lw.Close()
	return buf.Bytes()
}

// buildBzip2 compresses files into a .tar.bz2 archive using the system bzip2
// tool. The test is skipped if bzip2 is not installed.
// Note: stdlib compress/bzip2 is decode-only, so we need an external encoder.
func buildBzip2(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	_ = bzip2.NewReader // confirm import is used by production code
	raw := buildTar(t, files)
	compressed, err := bzip2CompressBytes(raw)
	if err != nil {
		t.Skipf("bzip2 tool unavailable — skipping bzip2 test: %v", err)
	}
	return compressed
}

// ---------------------------------------------------------------------------
// New (plain tar)
// ---------------------------------------------------------------------------

func TestNew_empty(t *testing.T) {
	if _, err := tarfs.New(nil); err == nil {
		t.Fatal("expected error for nil data")
	}
	if _, err := tarfs.New([]byte{}); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestNew_file(t *testing.T) {
	content := []byte("hello tar")
	tfs, err := tarfs.New(buildTar(t, map[string][]byte{"a.txt": content}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := fs.ReadFile(tfs, "a.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestNewFromReader(t *testing.T) {
	content := []byte("from reader")
	raw := buildTar(t, map[string][]byte{"r.txt": content})
	tfs, err := tarfs.NewFromReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("NewFromReader: %v", err)
	}
	got, _ := fs.ReadFile(tfs, "r.txt")
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

// ---------------------------------------------------------------------------
// NewGzip
// ---------------------------------------------------------------------------

func TestNewGzip_empty(t *testing.T) {
	if _, err := tarfs.NewGzip(nil); err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestNewGzip_file(t *testing.T) {
	content := []byte("hello gzip tar")
	tfs, err := tarfs.NewGzip(buildGzip(t, map[string][]byte{"g.txt": content}))
	if err != nil {
		t.Fatalf("NewGzip: %v", err)
	}
	got, err := fs.ReadFile(tfs, "g.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

// ---------------------------------------------------------------------------
// NewZstd
// ---------------------------------------------------------------------------

func TestNewZstd_empty(t *testing.T) {
	if _, err := tarfs.NewZstd(nil); err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestNewZstd_file(t *testing.T) {
	content := []byte("hello zstd tar")
	tfs, err := tarfs.NewZstd(buildZstd(t, map[string][]byte{"z.txt": content}))
	if err != nil {
		t.Fatalf("NewZstd: %v", err)
	}
	got, err := fs.ReadFile(tfs, "z.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

// ---------------------------------------------------------------------------
// fs.FS contract
// ---------------------------------------------------------------------------

func TestOpen_root(t *testing.T) {
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"a.txt": []byte("a")}))
	f, err := tfs.Open(".")
	if err != nil {
		t.Fatalf("Open(.): %v", err)
	}
	defer f.Close()
	info, _ := f.Stat()
	if !info.IsDir() || info.Name() != "." {
		t.Errorf("root: IsDir=%v Name=%q", info.IsDir(), info.Name())
	}
}

func TestOpen_notFound(t *testing.T) {
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"a.txt": []byte("a")}))
	_, err := tfs.Open("missing.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
}

func TestOpen_invalidPath(t *testing.T) {
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"a.txt": []byte("a")}))
	_, err := tfs.Open("/abs/path")
	if !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("want ErrInvalid, got %v", err)
	}
}

func TestStat(t *testing.T) {
	content := []byte("stat me")
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"sub/file.txt": content}))
	f, err := tfs.Open("sub/file.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	info, _ := f.Stat()
	if info.Name() != "file.txt" {
		t.Errorf("Name = %q, want file.txt", info.Name())
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("Size = %d, want %d", info.Size(), len(content))
	}
	if info.IsDir() {
		t.Error("IsDir should be false")
	}
}

func TestSeek(t *testing.T) {
	content := []byte("0123456789")
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"nums.txt": content}))
	f, _ := tfs.Open("nums.txt")
	defer f.Close()
	seeker := f.(io.ReadSeeker)
	if _, err := seeker.Seek(5, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 3)
	if _, err := io.ReadFull(f, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf) != "567" {
		t.Errorf("got %q, want 567", buf)
	}
}

func TestReadDir_sorted(t *testing.T) {
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{
		"dir/z.txt": []byte("z"),
		"dir/a.txt": []byte("a"),
		"dir/m.txt": []byte("m"),
	}))
	entries, err := tfs.ReadDir("dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	want := []string{"a.txt", "m.txt", "z.txt"}
	if len(entries) != len(want) {
		t.Fatalf("len(entries) = %d, want %d", len(entries), len(want))
	}
	for i, w := range want {
		if entries[i].Name() != w {
			t.Errorf("entries[%d] = %q, want %q", i, entries[i].Name(), w)
		}
	}
}

func TestReadFile_copy(t *testing.T) {
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"a.txt": []byte("original")}))
	b1, _ := tfs.ReadFile("a.txt")
	b1[0] = 'X'
	b2, _ := tfs.ReadFile("a.txt")
	if b2[0] != 'o' {
		t.Error("ReadFile must return a copy, but data was mutated")
	}
}

// ---------------------------------------------------------------------------
// Synthetic directory creation
// ---------------------------------------------------------------------------

func TestSyntheticDirs(t *testing.T) {
	// Archive with no explicit directory headers.
	tfs, err := tarfs.New(buildTar(t, map[string][]byte{
		"dist/assets/app.js": []byte("console.log('hi')"),
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, dir := range []string{"dist", "dist/assets"} {
		f, err := tfs.Open(dir)
		if err != nil {
			t.Fatalf("Open(%q): %v", dir, err)
		}
		info, _ := f.Stat()
		f.Close()
		if !info.IsDir() {
			t.Errorf("%q should be a synthetic directory", dir)
		}
	}
}

// ---------------------------------------------------------------------------
// Path normalization
// ---------------------------------------------------------------------------

func TestPathNormalization_dotSlash(t *testing.T) {
	// Manually build archive with "./dist/page.html" prefix.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "./dist/page.html", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("page"))
	_ = tw.Close()

	tfs, err := tarfs.New(buf.Bytes())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := tfs.ReadFile("dist/page.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "page" {
		t.Errorf("got %q, want page", got)
	}
}

// ---------------------------------------------------------------------------
// fs.Sub integration
// ---------------------------------------------------------------------------

func TestSub_integration(t *testing.T) {
	content := []byte("<html>sub</html>")
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"dist/index.html": content}))

	sub, err := fs.Sub(tfs, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	got, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("ReadFile via sub: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

// ---------------------------------------------------------------------------
// http.FileServer integration
// ---------------------------------------------------------------------------

func TestHTTPFileServer(t *testing.T) {
	content := []byte("<html>http</html>")
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"dist/index.html": content}))

	sub, _ := fs.Sub(tfs, "dist")
	srv := httptest.NewServer(http.FileServer(http.FS(sub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/index.html")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestHTTPFileServer_rangeRequest(t *testing.T) {
	content := []byte("0123456789abcdef")
	tfs, _ := tarfs.New(buildTar(t, map[string][]byte{"dist/data.bin": content}))

	sub, _ := fs.Sub(tfs, "dist")
	srv := httptest.NewServer(http.FileServer(http.FS(sub)))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/data.bin", nil)
	req.Header.Set("Range", "bytes=4-7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Errorf("status = %d, want 206", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "4567" {
		t.Errorf("range body = %q, want 4567", body)
	}
}

// ---------------------------------------------------------------------------
// NewLz4
// ---------------------------------------------------------------------------

func TestNewLz4_empty(t *testing.T) {
	if _, err := tarfs.NewLz4(nil); err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestNewLz4_file(t *testing.T) {
	content := []byte("hello lz4 tar")
	tfs, err := tarfs.NewLz4(buildLz4(t, map[string][]byte{"l.txt": content}))
	if err != nil {
		t.Fatalf("NewLz4: %v", err)
	}
	got, err := fs.ReadFile(tfs, "l.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestNewLz4_subAndHTTP(t *testing.T) {
	content := []byte("<html>lz4</html>")
	tfs, _ := tarfs.NewLz4(buildLz4(t, map[string][]byte{"dist/index.html": content}))
	sub, _ := fs.Sub(tfs, "dist")

	srv := httptest.NewServer(http.FileServer(http.FS(sub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/index.html")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Errorf("body = %q, want %q", body, content)
	}
}

// ---------------------------------------------------------------------------
// NewBzip2
// ---------------------------------------------------------------------------

func TestNewBzip2_empty(t *testing.T) {
	if _, err := tarfs.NewBzip2(nil); err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestNewBzip2_file(t *testing.T) {
	content := []byte("hello bzip2 tar")
	data := buildBzip2(t, map[string][]byte{"b.txt": content}) // skips if no bzip2 tool
	tfs, err := tarfs.NewBzip2(data)
	if err != nil {
		t.Fatalf("NewBzip2: %v", err)
	}
	got, err := fs.ReadFile(tfs, "b.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestNewBzip2_subAndHTTP(t *testing.T) {
	content := []byte("<html>bzip2</html>")
	data := buildBzip2(t, map[string][]byte{"dist/index.html": content})
	tfs, _ := tarfs.NewBzip2(data)
	sub, _ := fs.Sub(tfs, "dist")

	srv := httptest.NewServer(http.FileServer(http.FS(sub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/index.html")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Errorf("body = %q, want %q", body, content)
	}
}

// ---------------------------------------------------------------------------
// bzip2CompressBytes — encode using system bzip2 tool (stdlib is decode-only)
// ---------------------------------------------------------------------------

func bzip2CompressBytes(data []byte) ([]byte, error) {
	bzip2Path, err := exec.LookPath("bzip2")
	if err != nil {
		return nil, fmt.Errorf("bzip2 not found: %w", err)
	}
	cmd := exec.Command(bzip2Path, "--compress", "--stdout")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Output()
}
