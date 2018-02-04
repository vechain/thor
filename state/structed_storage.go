package state

import (
	"bytes"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// StructedStorage storage data type should implement this.
type StructedStorage interface {
	Encode() ([]byte, error)
	Decode([]byte) error
}

type hashStorage struct {
	thor.Hash
}

func (h *hashStorage) Encode() ([]byte, error) {
	if h.Hash.IsZero() {
		return nil, nil
	}
	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(h.Hash[:], "\x00"))
	return trimed, nil
}

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
