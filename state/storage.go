package state

import (
	"errors"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
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

func encodeUint(i uint64) ([]byte, error) {
	if i == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(i)
}
func encodeBytesTrimed(bs []byte) ([]byte, error) {
	var i int
	for ; i < len(bs); i++ {
		if bs[i] != 0 {
			break
		}
	}
	trimed := bs[i:]
	if len(trimed) == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(trimed)
}

func encodeString(str string) ([]byte, error) {
	if str == "" {
		return nil, nil
	}
	return rlp.EncodeToBytes(str)
}

func encodeStorage(val interface{}) ([]byte, error) {
	switch v := val.(type) {
	case thor.Bytes32:
		return encodeBytesTrimed(v[:])
	case *thor.Bytes32:
		return encodeBytesTrimed(v[:])
	case thor.Address:
		return encodeBytesTrimed(v[:])
	case *thor.Address:
		return encodeBytesTrimed(v[:])
	case string:
		return encodeString(v)
	case *string:
		return encodeString(*v)
	case uint:
		return encodeUint(uint64(v))
	case *uint:
		return encodeUint(uint64(*v))
	case uint8:
		return encodeUint(uint64(v))
	case *uint8:
		return encodeUint(uint64(*v))
	case uint16:
		return encodeUint(uint64(v))
	case *uint16:
		return encodeUint(uint64(*v))
	case uint32:
		return encodeUint(uint64(v))
	case *uint32:
		return encodeUint(uint64(*v))
	case uint64:
		return encodeUint(v)
	case *uint64:
		return encodeUint(*v)
	case *big.Int:
		if v.Sign() == 0 {
			return nil, nil
		}
		return rlp.EncodeToBytes(v)
	}
	return nil, errors.New("encode storage value: type " + reflect.TypeOf(val).String())
}

func decodeStorage(data []byte, val interface{}) error {
	switch v := val.(type) {
	case *thor.Bytes32:
		if len(data) == 0 {
			*v = thor.Bytes32{}
			return nil
		}
		_, content, _, err := rlp.Split(data)
		if err != nil {
			return err
		}
		(*common.Hash)(v).SetBytes(content)
		return nil
	case *thor.Address:
		if len(data) == 0 {
			*v = thor.Address{}
			return nil
		}
		_, content, _, err := rlp.Split(data)
		if err != nil {
			return err
		}
		(*common.Address)(v).SetBytes(content)
		return nil
	case *string:
		if len(data) == 0 {
			*v = ""
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *uint:
		if len(data) == 0 {
			*v = 0
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *uint8:
		if len(data) == 0 {
			*v = 0
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *uint16:
		if len(data) == 0 {
			*v = 0
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *uint32:
		if len(data) == 0 {
			*v = 0
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *uint64:
		if len(data) == 0 {
			*v = 0
			return nil
		}
		return rlp.DecodeBytes(data, v)
	case *big.Int:
		if len(data) == 0 {
			v.SetUint64(0)
			return nil
		}
		return rlp.DecodeBytes(data, v)
	}
	return errors.New("decode storage value: type " + reflect.TypeOf(val).String())
}
