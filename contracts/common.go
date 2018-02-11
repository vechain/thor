package contracts

import (
	"bytes"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/contracts/gen"
	"github.com/vechain/thor/poa"
)

func mustLoadABI(assetName string) *abi.ABI {
	data := gen.MustAsset(assetName)
	abi, err := abi.JSON(bytes.NewReader(data))
	if err != nil {
		panic(errors.Wrap(err, "load ABI"))
	}
	return &abi
}

func mustLoadHexData(assetName string) []byte {
	data, err := hex.DecodeString(string(gen.MustAsset(assetName)))
	if err != nil {
		panic(errors.Wrap(err, "load runtime byte code"))
	}
	return data
}

func mustPack(abi *abi.ABI, name string, args ...interface{}) []byte {
	data, err := abi.Pack(name, args...)
	if err != nil {
		panic(errors.Wrap(err, "pack "+name))
	}
	return data
}

func mustUnpack(abi *abi.ABI, v interface{}, name string, output []byte) {
	if err := abi.Unpack(v, name, output); err != nil {
		panic(errors.Wrap(err, "unpack "+name))
	}
}

var errNativeNotPermitted = errors.New("native: not permitted")

//////

/////
type stgProposer poa.Proposer

func (s *stgProposer) Encode() ([]byte, error) {
	if s.Address.IsZero() && s.Status == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(s)
}

func (s *stgProposer) Decode(data []byte) error {
	if len(data) == 0 {
		*s = stgProposer{}
		return nil
	}
	return rlp.DecodeBytes(data, s)
}
