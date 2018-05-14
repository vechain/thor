package txpool

import (
	"math/big"

	"github.com/vechain/thor/thor"

	"github.com/vechain/thor/chain"

	"github.com/ethereum/go-ethereum/log"
	"github.com/vechain/thor/tx"
)

type objectStatus uint

const (
	Pending objectStatus = iota
	Queued
)

//txObject wrap transaction
type txObject struct {
	tx           *tx.Transaction
	signer       thor.Address
	status       objectStatus
	overallGP    *big.Int
	creationTime int64
	deleted      bool
}

func (txObjs *txObject) currentState(chain *chain.Chain, bestBlockNum uint32) objectStatus {
	dependsOn := txObjs.tx.DependsOn()
	if dependsOn != nil {
		if _, _, err := chain.GetTransaction(*dependsOn); err != nil {
			if !chain.IsNotFound(err) {
				log.Error("err", err)
			}
			return Queued
		}
	}

	if txObjs.tx.BlockRef().Number() > bestBlockNum+1 {
		return Queued
	}

	return Pending
}

type txObjects []*txObject

func (txObjs txObjects) parseTxs() []*tx.Transaction {
	txs := make(tx.Transactions, 0, len(txObjs))
	for _, obj := range txObjs {
		if !obj.deleted {
			txs = append(txs, obj.tx)
		}
	}
	return txs
}
