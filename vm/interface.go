package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

// ContractRef is a reference to the contract's backing object.
type ContractRef interface {
	Address() acc.Address
}

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	Engine() consensus.Engine
	GetHeader(cry.Hash, uint64) *types.Header
}

// Message represents a message sent to a contract.
type Message interface {
	From() acc.Address
	To() *acc.Address
	GasPrice() *big.Int
	Gas() *big.Int
	Value() *big.Int
	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// StateReader is account.StateReader's facade.
type StateReader interface {
	GetAccout(acc.Address) *acc.Account // if don't have, return nil
	GetStorage(cry.Hash) cry.Hash
}

// KVReader is account.KVReader's facade.
type KVReader interface {
	GetValue(cry.Hash) []byte // if don't have, return nil
}
