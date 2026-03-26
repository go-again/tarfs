package tarfs

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// NewZstd builds an FS from a zstd-compressed tar archive (.tar.zst).
// data is typically an []byte variable populated with go:embed.
func NewZstd(data []byte) (*FS, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tarfs: empty archive")
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("tarfs: zstd reader: %w", err)
	}
	defer dec.Close()

	raw, err := dec.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("tarfs: zstd decode: %w", err)
	}
	return New(raw)
}
