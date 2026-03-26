package tarfs

import (
	"bytes"
	"compress/bzip2"
	"fmt"
)

// NewBzip2 builds an FS from a bzip2-compressed tar archive (.tar.bz2 / .tbz2).
// data is typically an []byte variable populated with go:embed.
//
// Note: bzip2 decompression is slower than gzip or zstd. For new archives,
// prefer [NewGzip] or [NewZstd].
func NewBzip2(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	return NewFromReader(bzip2.NewReader(bytes.NewReader(data)))
}
