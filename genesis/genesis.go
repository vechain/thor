package genesis

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
)

type Builder struct {
	block.Builder

	allocs []alloc
}

type alloc struct {
	address   acc.Address
	balance   *big.Int
	bytecodes []byte
}

func (b *Builder) Alloc(addr acc.Address, balance *big.Int, bytecodes []byte) *Builder {
	b.allocs = append(b.allocs, alloc{
		addr,
		balance,
		bytecodes,
	})
	return b
}

func (b *Builder) Build(state State) *block.Block {
	return nil
}
