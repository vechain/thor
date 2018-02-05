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

type hashStorage struct {
	thor.Hash
}

var _ StorageEncoder = (*hashStorage)(nil)
var _ StorageDecoder = (*hashStorage)(nil)

// implements StorageEncoder.
func (h *hashStorage) Encode() ([]byte, error) {
	if h.Hash.IsZero() {
		return nil, nil
	}
	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(h.Hash[:], "\x00"))
	return trimed, nil
}

// implements StorageDecoder.
func (h *hashStorage) Decode(data []byte) error {
	if len(data) == 0 {
		h.Hash = thor.Hash{}
		return nil
	}
	_, content, _, err := rlp.Split(data)
	if err != nil {
		return err
	}
	h.Hash = thor.BytesToHash(content)
	return nil
}
