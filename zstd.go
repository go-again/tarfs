package tarfs

import (
	"bytes"
	"fmt"

	"github.com/go-again/az"
)

// NewZstd builds an FS from a zstd-compressed tar archive (.tar.zst).
// data is typically an []byte variable populated with go:embed.
// After this call returns, data is no longer referenced by the FS and may be
// nilled out to drop the Go reference to the compressed bytes.
func NewZstd(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	r := az.NewReader(bytes.NewReader(data))
	defer r.Close()
	return NewFromReader(r)
}
