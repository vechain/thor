package contracts

import (
	"bytes"
	"encoding/hex"

	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/contracts/gen"
	"github.com/vechain/thor/thor"
)

// all info about a contract.
type contract struct {
	Address         thor.Address // pre-alloced address
	ABI             abi.ABI
	binRuntimeAsset string
}

// load contract info from asset.
// panic if failed.
func mustLoad(addr thor.Address, abiAsset, binRuntimeAsset string) contract {
	data := gen.MustAsset(abiAsset)
	abi, err := abi.JSON(bytes.NewReader(data))
	if err != nil {
		panic(errors.Wrap(err, "load ABI"))
	}
	return contract{
		addr,
		abi,
		binRuntimeAsset,
	}
}

// RuntimeBytecodes returns runtime bytecodes of the contract.
// panic if failed.
func (c *contract) RuntimeBytecodes() []byte {
	data, err := hex.DecodeString(string(gen.MustAsset(c.binRuntimeAsset)))
	if err != nil {
		panic(errors.Wrap(err, "load runtime byte code"))
	}
	return data
}
