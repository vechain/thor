package state

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// StorageCodec to encode or decode raw storage value.
type StorageCodec interface {
	Decode([]byte) (interface{}, error)
	Encode(interface{}) ([]byte, error)

	Default() interface{}
}

// HashStorageCodec a codec for encoding or decoding raw storage value to Hash type.
var HashStorageCodec StorageCodec = &hashStorageCodec{}

type hashStorageCodec struct {
}

func (h *hashStorageCodec) Decode(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return thor.Hash{}, nil
	}
	_, content, _, err := rlp.Split(data)
	if err != nil {
		return thor.Hash{}, err
	}

	return thor.BytesToHash(content), nil
}
func (h *hashStorageCodec) Encode(value interface{}) ([]byte, error) {
	v, ok := value.(thor.Hash)
	if !ok {
		return nil, errors.New("value must be hash type")
	}
	if (thor.Hash{}) == v {
		return nil, nil
	}

	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(v[:], "\x00"))
	return trimed, nil
}

func (h *hashStorageCodec) Default() interface{} {
	return thor.Hash{}
}
