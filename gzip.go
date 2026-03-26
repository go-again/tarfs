package tarfs

import (
	"bytes"
	"compress/gzip"
	"fmt"
)

// NewGzip builds an FS from a gzip-compressed tar archive (.tar.gz / .tgz).
// data is typically an []byte variable populated with go:embed.
func NewGzip(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("tarfs: gzip reader: %w", err)
	}
	defer gr.Close()
	return NewFromReader(gr)
}
