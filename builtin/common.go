package builtin

import (
	"bytes"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/vechain/thor/builtin/abi"
	"github.com/vechain/thor/builtin/gen"
	"github.com/vechain/thor/thor"
)

type contract struct {
	name    string
	Address thor.Address
	ABI     *abi.ABI
}

func loadContract(name string) *contract {
	asset := "compiled/" + name + ".abi"
	data := gen.MustAsset(asset)
	abi, err := abi.New(bytes.NewReader(data))
	if err != nil {
		panic(errors.Wrap(err, "load ABI for '"+name+"'"))
	}

	return &contract{
		name,
		thor.BytesToAddress([]byte(name)),
		abi,
	}
}

// RuntimeBytecodes load runtime byte codes.
func (c *contract) RuntimeBytecodes() []byte {
	asset := "compiled/" + c.name + ".bin-runtime"
	data, err := hex.DecodeString(string(gen.MustAsset(asset)))
	if err != nil {
		panic(errors.Wrap(err, "load runtime byte code for '"+c.name+"'"))
	}
	return data
}
