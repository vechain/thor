package vm

import (
	"math/big"

	"github.com/vechain/thor/cry"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/vm/evm"
)

// Context is ref to evm.Context.
type Context evm.Context

// NewEVMContext return a new evm.Context.
// origin: Message.From()
// price: Message.Price()
// txHash: Message.TransactionHash()
func NewEVMContext(header *block.Header, price *big.Int, origin acc.Address, txHash cry.Hash, getHash func(uint64) cry.Hash) Context {
	tGetHash := func(n uint64) common.Hash {
		return common.Hash(getHash(n))
	}

	return Context{
		CanTransfer: canTransfer,
		Transfer:    transfer,
		GetHash:     tGetHash,
		Origin:      common.Address(origin),
		Coinbase:    common.Address(header.Beneficiary()),
		BlockNumber: new(big.Int).SetUint64(uint64(header.Number())),
		Time:        new(big.Int).SetUint64(header.Timestamp()),
		Difficulty:  new(big.Int),
		GasLimit:    header.GasLimit(),
		GasPrice:    price,
		TxHash:      common.Hash(txHash),
	}
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
