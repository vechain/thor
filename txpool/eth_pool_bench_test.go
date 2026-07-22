// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"testing"
	"time"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func BenchmarkEthPoolMapWashDefaultLimit(b *testing.B) {
	const (
		senderCount = 125
		pendingPer  = 16
		queuedPer   = 64
	)
	costs := newCostTracker()
	poolMap := newEthPoolMap(costs)
	stateNonces := make(map[thor.Address]uint64, senderCount)
	origins := make(map[*TxObject]thor.Address, senderCount*(pendingPer+queuedPer))
	balance := big.NewInt(1_000_000)

	for senderIndex := range senderCount {
		var origin thor.Address
		origin[0] = byte(senderIndex)
		origin[1] = byte(senderIndex >> 8)
		sender := newEthSender(origin, 0)
		stateNonces[origin] = 0
		for nonce := range pendingPer + queuedPer {
			trx := tx.NewBuilder(tx.TypeEthDynamicFee).
				ChainID(uint64(senderIndex + 1)).
				Nonce(uint64(nonce)).
				MaxFeePerGas(big.NewInt(2)).
				MaxPriorityFeePerGas(big.NewInt(1)).
				Build()
			txObj := &TxObject{
				Transaction:      trx,
				timeAdded:        time.Now().UnixNano(),
				priorityGasPrice: big.NewInt(1),
				executable:       nonce < pendingPer,
			}
			origins[txObj] = origin
			poolMap.allByHash[txObj.Hash()] = txObj
			if nonce < pendingPer {
				sender.pending[uint64(nonce)] = txObj
				if err := costs.reserve(
					ethReservationOwner(origin, uint64(nonce)),
					origin,
					big.NewInt(1),
					balance,
				); err != nil {
					b.Fatal(err)
				}
			} else {
				sender.queue[uint64(nonce)] = txObj
			}
		}
		poolMap.senders[origin] = sender
	}
	prepare := func(txObj *TxObject) (reservationRequest, bool, error) {
		origin := origins[txObj]
		return reservationRequest{
			owner:   ethReservationOwner(origin, txObj.Nonce()),
			payer:   origin,
			cost:    big.NewInt(1),
			balance: balance,
		}, true, nil
	}
	options := ethWashOptions{
		pendingLimit: pendingPer,
		queueLimit:   queuedPer,
		globalLimit:  senderCount * (pendingPer + queuedPer),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := poolMap.wash(stateNonces, options, prepare); err != nil {
			b.Fatal(err)
		}
	}
}
