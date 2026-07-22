// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func benchmarkEthMapObject(b *testing.B, nonce uint64) *TxObject {
	b.Helper()
	to := devAccounts[1].Address
	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(1).
		Nonce(nonce).
		Gas(21_000).
		MaxFeePerGas(big.NewInt(100)).
		MaxPriorityFeePerGas(big.NewInt(10)).
		To(&to).
		Build(), devAccounts[0].PrivateKey)
	txObj, err := ResolveTx(trx, false)
	if err != nil {
		b.Fatal(err)
	}
	return txObj
}

func benchmarkEthPrepare(txObj *TxObject) (reservationRequest, bool, error) {
	payer := txObj.Origin()
	return reservationRequest{
		owner:   ethReservationOwner(payer, txObj.Nonce()),
		payer:   payer,
		cost:    big.NewInt(1),
		balance: big.NewInt(1_000_000),
	}, true, nil
}

func benchmarkPopulatedEthMap(b *testing.B) *ethPoolMap {
	b.Helper()
	poolMap := newEthPoolMap(newCostTracker())
	for nonce := uint64(1); nonce <= 80; nonce++ {
		txObj := benchmarkEthMapObject(b, nonce)
		if _, _, err := poolMap.add(txObj, 0, 0, 16, 1_000, 10, benchmarkEthPrepare); err != nil {
			b.Fatal(err)
		}
	}
	return poolMap
}

func BenchmarkEthPoolMapAdd(b *testing.B) {
	poolMap := benchmarkPopulatedEthMap(b)
	candidate := benchmarkEthMapObject(b, 1_000)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := poolMap.add(candidate, 0, 0, 16, 1_000, 10, benchmarkEthPrepare); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		poolMap.removeByHash(candidate.Hash())
		b.StartTimer()
	}
}

func BenchmarkEthPoolMapAddParallel(b *testing.B) {
	poolMap := benchmarkPopulatedEthMap(b)
	candidates := make([]*TxObject, 256)
	for i := range candidates {
		candidates[i] = benchmarkEthMapObject(b, uint64(2_000+i))
	}
	var cursor atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			candidate := candidates[cursor.Add(1)%uint64(len(candidates))]
			_, _, _ = poolMap.add(candidate, 0, 0, 16, 1_000, 10, benchmarkEthPrepare)
			poolMap.removeByHash(candidate.Hash())
		}
	})
}

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
