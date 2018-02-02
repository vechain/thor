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

// Predefined storage codec
var (
	HashStorageCodec    StorageCodec = &hashCodec{}
	AddressStorageCodec StorageCodec = &addressCodec{}
)

type hashCodec struct{}

func (h *hashCodec) Decode(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return thor.Hash{}, nil
	}
	_, content, _, err := rlp.Split(data)
	if err != nil {
		return thor.Hash{}, err
	}

	return thor.BytesToHash(content), nil
}

func (h *hashCodec) Encode(value interface{}) ([]byte, error) {
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

func (h *hashCodec) Default() interface{} {
	return thor.Hash{}
}

////
type addressCodec struct{}

func (a *addressCodec) Decode(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return thor.Address{}, nil
	}
	_, content, _, err := rlp.Split(data)
	if err != nil {
		return thor.Address{}, err
	}

	return thor.BytesToAddress(content), nil
}

func (a *addressCodec) Encode(value interface{}) ([]byte, error) {
	v, ok := value.(thor.Address)
	if !ok {
		return nil, errors.New("value must be address type")
	}
	if (thor.Address{}) == v {
		return nil, nil
	}

	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(v[:], "\x00"))
	return trimed, nil
}

func (a *addressCodec) Default() interface{} {
	return thor.Address{}
}
