package block

import (
	"math/big"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

type Header struct {
	ParentHash      cry.Hash
	Timestamp       uint64
	GasLimit        *big.Int
	GasUsed         *big.Int
	RewardRecipient *acc.Address

	TxsRoot      cry.Hash
	StateRoot    cry.Hash
	ReceiptsRoot cry.Hash
}

func (h Header) Copy() *Header {
	if h.GasLimit == nil {
		h.GasLimit = new(big.Int)
	} else {
		h.GasLimit = new(big.Int).Set(h.GasLimit)
	}

	if h.GasUsed == nil {
		h.GasUsed = new(big.Int)
	} else {
		h.GasUsed = new(big.Int).Set(h.GasUsed)
	}

	if h.RewardRecipient != nil {
		cpy := *h.RewardRecipient
		h.RewardRecipient = &cpy
	}
	return &h
}
