# tarfs

A read-only, in-memory [`fs.FS`](https://pkg.go.dev/io/fs#FS) backed by a tar archive.

Embed a compressed archive into your binary with `//go:embed`, pass it to `tarfs.NewAz`, `tarfs.NewZstd`, or `tarfs.NewGzip`, and serve the result directly with `http.FileServer` or any function that accepts `fs.FS`.

- Supports plain tar, gzip (`.tar.gz`), zstd (`.tar.zst`), lz4 (`.tar.lz4`), and az (`.tar.az`) — pick whatever suits your size vs. speed needs.
- `NewAz` auto-detects lz4 or zstd from the stream header — one constructor, either format.
- Decompresses once at startup; all subsequent reads are served from memory with no further CPU cost.
- Implements `fs.FS`, `fs.ReadDirFS`, `fs.ReadFileFS`, and `io.ReadSeeker` on file handles (needed for HTTP range requests).
- Synthesizes directory entries — works even when the archive omits explicit directory headers.
- `./`-prefixed paths (produced by `tar cf`) are normalized automatically.

## Installation

```sh
go get github.com/go-again/tarfs
```

> **Dependencies**: the base package uses only the Go standard library.
> `NewAz`, `NewZstd`, and `NewLz4` pull in `github.com/go-again/az` (no transitive deps).
> `NewGzip` and `NewBzip2` use only stdlib.

## Quick start

### With zstd

The smallest binary for a large asset bundle. Requires `github.com/go-again/az`.

**1. Build the archive**

```sh
# Pack the dist/ directory into a compressed archive.
# The dist/ prefix is preserved so fs.Sub can strip it later.
tar -cf - dist/ | zstd -19 -o assets.tar.zst
```

**2. Embed and serve**

```go
package main

import (
    _ "embed"
    "io/fs"
    "net/http"

    "github.com/go-again/tarfs"
)

//go:embed assets.tar.zst
var assetData []byte

func main() {
    tfs, err := tarfs.NewZstd(assetData)
    if err != nil {
        panic(err)
    }

    // Strip the "dist/" prefix so URLs map to /index.html, not /dist/index.html.
    sub, _ := fs.Sub(tfs, "dist")

    http.ListenAndServe(":8080", http.FileServer(http.FS(sub)))
}
```

### With gzip

No extra dependencies — uses the Go standard library's `compress/gzip`.

**1. Build the archive**

```sh
tar -czf assets.tar.gz dist/
```

**2. Embed and serve**

```go
//go:embed assets.tar.gz
var assetData []byte

tfs, err := tarfs.NewGzip(assetData)
```

Everything else is identical to the zstd example.

### With lz4

Fastest decompression of any supported format — good when startup latency matters.

**1. Build the archive**

```sh
# requires lz4 CLI: apt install lz4 / brew install lz4
tar -cf - dist/ | lz4 -9 > assets.tar.lz4
```

**2. Embed and serve**

```go
//go:embed assets.tar.lz4
var assetData []byte

tfs, err := tarfs.NewLz4(assetData)
```

### With az (auto-detect lz4 or zstd)

The [`az` CLI](https://pkg.go.dev/github.com/go-again/az/cmd/az) is a single tool that covers both lz4 (levels 1–2) and zstd (levels 3–5). The `NewAz` constructor auto-detects the format from the stream header, so you can switch compression levels without changing any Go code.

**Install the az CLI**

```sh
go install github.com/go-again/az/cmd/az@latest
```

**1. Build the archive**

```sh
# -5: zstd best compression
tar -cf - dist/ | az -5 > assets.tar.az

# -1: lz4 fastest decompression
tar -cf - dist/ | az -1 > assets.tar.az
```

**2. Embed and serve**

```go
//go:embed assets.tar.az
var assetData []byte

tfs, err := tarfs.NewAz(assetData)
```

`NewAz` also accepts existing `.tar.lz4` and `.tar.zst` archives — format detection is based on magic bytes, not file extension.

### With bzip2

For reading existing `.tar.bz2` / `.tbz2` archives. Uses stdlib `compress/bzip2` (decode-only — no extra dependency).

> Note: bzip2 is slower to decompress than gzip or zstd. For new archives, prefer `NewGzip` or `NewZstd`.

**1. Build the archive**

```sh
tar -cjf assets.tar.bz2 dist/
```

**2. Embed and serve**

```go
//go:embed assets.tar.bz2
var assetData []byte

tfs, err := tarfs.NewBzip2(assetData)
```

### With plain tar (no compression)

```sh
tar -cf assets.tar dist/
```

```go
//go:embed assets.tar
var assetData []byte

tfs, err := tarfs.New(assetData)
```

### From an io.Reader

If you receive a raw tar stream from a network call, a file, or another decompressor:

```go
import (
    "compress/bzip2"
    "os"

    "github.com/go-again/tarfs"
)

f, _ := os.Open("assets.tar.bz2")
defer f.Close()

tfs, err := tarfs.NewFromReader(bzip2.NewReader(f))
```

## Production pattern

In a real application the FS is typically built once at startup and reused for
the lifetime of the process. Use `sync.Once` to guarantee that, and nil out the
raw archive bytes immediately after construction to drop the Go reference to the
compressed data:

```go
import (
    _ "embed"
    "io/fs"
    "sync"

    "github.com/go-again/tarfs"
)

//go:embed assets.tar.zst
var assetData []byte

var (
    assetOnce sync.Once
    assetFS   fs.FS
)

func Assets() fs.FS {
    assetOnce.Do(func() {
        tfs, err := tarfs.NewZstd(assetData)
        assetData = nil // drop reference to compressed bytes
        if err != nil {
            return // assetFS stays nil; caller handles gracefully
        }
        assetFS = tfs
    })
    return assetFS
}
```

**Why nil `assetData`?** `//go:embed` stores the compressed bytes in the
binary's read-only data segment — the Go GC cannot free them. However, dropping
the Go reference lets the OS page out those pages under memory pressure once the
data has been fully decompressed into the heap by `tarfs`. The decompressed
in-memory copy in `tfs` is what actually gets served.

## Serving a single-page application

SPAs need a fallback to `index.html` for client-side routes. A thin wrapper around `http.FileServer` handles this:

```go
sub, _ := fs.Sub(tfs, "dist")
fileServer := http.FileServer(http.FS(sub))

http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    // Try to serve the actual file.
    path := r.URL.Path
    if path == "/" {
        path = "index.html"
    } else {
        path = path[1:] // strip leading slash
    }

    if _, err := fs.Stat(sub, path); err == nil {
        fileServer.ServeHTTP(w, r)
        return
    }

    // Fallback to index.html for SPA routing.
    r.URL.Path = "/"
    fileServer.ServeHTTP(w, r)
})
```

## Reading individual files

`*tarfs.FS` implements `fs.ReadFileFS`, so you can read files directly without opening a handle:

```go
data, err := tfs.ReadFile("dist/config.json")
```

Or use the stdlib helpers:

```go
data, err := fs.ReadFile(tfs, "dist/config.json")
```

## Listing directories

`*tarfs.FS` implements `fs.ReadDirFS`. Entries are always returned in alphabetical order.

```go
entries, err := tfs.ReadDir("dist/assets")
for _, e := range entries {
    fmt.Println(e.Name(), e.IsDir())
}
```

Or with `fs.Sub`:

```go
sub, _ := fs.Sub(tfs, "dist")
entries, _ := fs.ReadDir(sub, "assets")
```

## Dev-mode placeholder

`//go:embed` requires the file to exist at compile time. Commit a zero-byte placeholder so `go build` works without running the asset pipeline:

```sh
touch assets.tar.zst   # zero bytes — tarfs.NewZstd returns an error gracefully
```

In your application, handle the error:

```go
tfs, err := tarfs.NewZstd(assetData)
if err != nil {
    log.Warn("assets not built — UI unavailable")
    // continue without serving static files
}
```

## Archive path layout

The archive can contain a top-level prefix (e.g. `dist/`) or not — both work.

**With prefix** — use `fs.Sub` to strip it:

```sh
tar -czf assets.tar.gz dist/   # paths: dist/index.html, dist/assets/app.js
```

```go
tfs, _ := tarfs.NewGzip(assetData)
sub, _ := fs.Sub(tfs, "dist")  // now: index.html, assets/app.js
```

**Without prefix** — use the FS directly:

```sh
tar -czf assets.tar.gz -C dist/ .  # paths: index.html, assets/app.js
```

```go
tfs, _ := tarfs.NewGzip(assetData)
// Open("index.html") works directly
```

## API reference

```go
// New builds an FS from raw (uncompressed) tar bytes.
func New(data []byte) (*FS, error)

// NewFromReader builds an FS from a raw tar io.Reader.
// Wrap with your decompressor before calling for compressed streams.
func NewFromReader(r io.Reader) (*FS, error)

// NewGzip builds an FS from a gzip-compressed tar archive (.tar.gz / .tgz).
// Uses stdlib compress/gzip — no extra dependency.
func NewGzip(data []byte) (*FS, error)

// NewBzip2 builds an FS from a bzip2-compressed tar archive (.tar.bz2 / .tbz2).
// Uses stdlib compress/bzip2 — no extra dependency.
func NewBzip2(data []byte) (*FS, error)

// NewAz builds an FS from an az-compressed tar archive (.tar.az).
// Auto-detects lz4 (levels 1–2) or zstd (levels 3–5) from the stream header.
// Also accepts existing .tar.lz4 and .tar.zst archives.
// Requires github.com/go-again/az.
func NewAz(data []byte) (*FS, error)

// NewZstd builds an FS from a zstd-compressed tar archive (.tar.zst).
// Requires github.com/go-again/az.
func NewZstd(data []byte) (*FS, error)

// NewLz4 builds an FS from an lz4-compressed tar archive (.tar.lz4).
// Requires github.com/go-again/az.
func NewLz4(data []byte) (*FS, error)

// Open implements fs.FS.
func (f *FS) Open(name string) (fs.File, error)

// ReadDir implements fs.ReadDirFS. Entries are sorted alphabetically.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error)

// ReadFile implements fs.ReadFileFS. Returns a copy of the file contents.
func (f *FS) ReadFile(name string) ([]byte, error)
```

File handles returned by `Open` implement:
- `fs.File` (`Read`, `Stat`, `Close`)
- `io.Seeker` — enables HTTP range requests via `http.FileServer`
- `fs.ReadDirFile` (`ReadDir`) — required by `http.FileServer` for directory nodes

## Compression comparison

| Format    | Extension   | Extra dep         | CLI levels | Compression ratio | Decompression speed |
|-----------|-------------|-------------------|------------|-------------------|---------------------|
| none      | `.tar`      | none              | —          | none              | instant             |
| gzip      | `.tar.gz`   | none (stdlib)     | —          | moderate          | fast                |
| bzip2     | `.tar.bz2`  | none (decode-only)| —          | good              | slow                |
| az / lz4  | `.tar.az`   | `go-again/az`     | `-1`, `-2` | low–moderate      | fastest             |
| az / zstd | `.tar.az`   | `go-again/az`     | `-3` to `-5` | best            | very fast           |

**Recommendations:**
- **New archives** — use zstd (`zstd -19` or `az -5`): best ratio, fast to decompress
- **Startup-sensitive** — use lz4 (`lz4 -9` or `az -1`): decompresses ~2–3× faster than gzip, ~1.5× faster than zstd
- **Single CLI for both** — use `az` + `NewAz`: one tool, one constructor, switch levels without changing Go code
- **Legacy / existing archives** — use the matching constructor; `NewFromReader` handles any other compressor
- **Zero extra deps** — use gzip; bzip2 only if you already have the archive

## Sponsors

tarfs is sponsored by [ssh2incus](https://ssh2incus.com) — an SSH gateway for [Incus](https://github.com/ssh2incus/ssh2incus) containers and VMs. ssh2incus uses tarfs to embed its web UI frontend into a single self-contained binary.

## License

MIT
