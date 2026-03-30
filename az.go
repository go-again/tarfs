package tarfs

import (
	"bytes"
	"fmt"

	"github.com/go-again/az"
)

// NewAz builds an FS from an az-compressed tar archive (.tar.az).
// It auto-detects whether the stream uses the lz4 format (levels 1–2) or
// the zstd format (levels 3–5) by inspecting the 4-byte magic header.
//
// This is the recommended constructor when using the [az CLI]:
//
//	tar -cf - dist/ | az -5 > assets.tar.az   # zstd level 5 (best compression)
//	tar -cf - dist/ | az -1 > assets.tar.az   # lz4 level 1 (fastest)
//
// NewAz also accepts existing .tar.lz4 and .tar.zst archives — the format
// is detected automatically regardless of the file extension.
//
// [az CLI]: https://pkg.go.dev/github.com/go-again/az/cmd/az
func NewAz(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	r := az.NewReader(bytes.NewReader(data))
	defer r.Close()
	return NewFromReader(r)
}
