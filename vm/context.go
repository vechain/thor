package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/vechain/vecore/acc"
)

// Context is ref to vm.Context.
type Context vm.Context

// NewEVMContext return a new vm.Context.
func NewEVMContext(msg Message, header *types.Header, chain ChainContext, author *acc.Address) Context {
	return Context(core.NewEVMContext(&vmMessage{msg}, header, &vmChainContext{chain}, (*common.Address)(author)))
}
