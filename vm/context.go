package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm/evm"

	"github.com/vechain/thor/acc"
)

// Context for VM runtime.
type Context struct {
	Origin      acc.Address
	Beneficiary acc.Address
	BlockNumber *big.Int
	Time        *big.Int
	GasLimit    *big.Int
	GasPrice    *big.Int
	TxHash      cry.Hash
	GetHash     func(uint64) cry.Hash
}

// CanTransfer checks wether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas in to account to make the transfer valid.
func canTransfer(db evm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func transfer(db evm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}
