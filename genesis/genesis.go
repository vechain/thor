package genesis

import (
	"encoding/hex"

	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type Genesis struct {
	builder *Builder
	id      thor.Bytes32
}

func (g *Genesis) Build(stateCreator *state.Creator) (blk *block.Block, logs []*tx.Log, err error) {
	return g.builder.Build(stateCreator)
}

func (g *Genesis) ID() thor.Bytes32 {
	return g.id
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m := abi.MethodByName(name)
	if m == nil {
		panic("no method '" + name + "'")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}

func mustDecodeHex(str string) []byte {
	data, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}
	return data
}

var emptyRuntimeBytecode = mustDecodeHex("6060604052600080fd00a165627a7a72305820c23d3ae2dc86ad130561a2829d87c7cb8435365492bd1548eb7e7fc0f3632be90029")
