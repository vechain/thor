package state

import (
	"bytes"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// StorageEncoder implement it to customize enconding process for storage data.
type StorageEncoder interface {
	Encode() ([]byte, error)
}

// StorageDecoder implement it to customize decoding process for storage data.
type StorageDecoder interface {
	Decode([]byte) error
}

type stgHash thor.Hash

var _ StorageEncoder = (*stgHash)(nil)
var _ StorageDecoder = (*stgHash)(nil)

// implements StorageEncoder.
func (h *stgHash) Encode() ([]byte, error) {
	if (*thor.Hash)(h).IsZero() {
		return nil, nil
	}
	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(h[:], "\x00"))
	return trimed, nil
}

// implements StorageDecoder.
func (h *stgHash) Decode(data []byte) error {
	if len(data) == 0 {
		*h = stgHash{}
		return nil
	}
	_, content, _, err := rlp.Split(data)
	if err != nil {
		return err
	}
	*h = stgHash(thor.BytesToHash(content))
	return nil
}
