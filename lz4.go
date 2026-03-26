package tarfs

import (
	"bytes"
	"fmt"

	"github.com/pierrec/lz4/v4"
)

// NewLz4 builds an FS from an lz4-compressed tar archive (.tar.lz4).
// data is typically an []byte variable populated with go:embed.
// After this call returns, data is no longer referenced by the FS and may be
// nilled out to drop the Go reference to the compressed bytes.
func NewLz4(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	return NewFromReader(lz4.NewReader(bytes.NewReader(data)))
}
