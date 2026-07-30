// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func benchmarkEthCoreObject(b *testing.B, nonce uint64) *TxObject {
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

func benchmarkEthPrepare(txObj *TxObject) ethPreparation {
	payer := txObj.Origin()
	return ethPreparation{
		request: reservationRequest{
			owner:   ethReservationOwner(payer, txObj.Nonce()),
			payer:   payer,
			cost:    big.NewInt(1),
			balance: big.NewInt(1_000_000),
		},
		viable: true,
	}
}

func benchmarkPopulatedEthCore(b *testing.B) *ethPoolCore {
	b.Helper()
	core := newEthPoolCore(newCostTracker(), Options{})
	for nonce := uint64(1); nonce <= 80; nonce++ {
		txObj := benchmarkEthCoreObject(b, nonce)
		if _, _, err := core.add(txObj, 0, 0, 16, 1_000, 10, benchmarkEthPrepare); err != nil {
			b.Fatal(err)
		}
	}
	return core
}

func BenchmarkEthPoolCoreAdd(b *testing.B) {
	core := benchmarkPopulatedEthCore(b)
	candidate := benchmarkEthCoreObject(b, 1_000)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := core.add(candidate, 0, 0, 16, 1_000, 10, benchmarkEthPrepare); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		core.removeByHash(candidate.Hash())
		b.StartTimer()
	}
}

func BenchmarkEthPoolCoreAddParallel(b *testing.B) {
	core := benchmarkPopulatedEthCore(b)
	candidates := make([]*TxObject, 256)
	for i := range candidates {
		candidates[i] = benchmarkEthCoreObject(b, uint64(2_000+i))
	}
	var cursor atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			candidate := candidates[cursor.Add(1)%uint64(len(candidates))]
			_, _, _ = core.add(candidate, 0, 0, 16, 1_000, 10, benchmarkEthPrepare)
			core.removeByHash(candidate.Hash())
		}
	})
}

func BenchmarkEthPoolCoreReadersDuringSlowPrepare(b *testing.B) {
	options := Options{
		EthAccountSlots: 16,
		EthAccountQueue: 64,
		Limit:           125,
	}
	core := newEthPoolCore(newCostTracker(), options)
	candidate := benchmarkEthCoreObject(b, 0)
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	prepare := func(txObj *TxObject) ethPreparation {
		time.Sleep(50 * time.Microsecond)
		return benchmarkEthPrepare(txObj)
	}

	go func() {
		defer close(writerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _, _ = core.add(candidate, 0, 1, 16, 64, 10, prepare)
			core.removeByHash(candidate.Hash())
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = core.Len()
		_ = core.executableSnapshot()
	}
	b.StopTimer()
	close(stop)
	<-writerDone
}

// benchmarkEthCorePool builds a core holding senderCount senders with pendingPer
// executable and queuedPer queued transactions each. It populates the indexes
// directly so setup cost does not depend on the path under benchmark, and hands
// back a prepare that reads no chain state, isolating in-core work.
func benchmarkEthCorePool(
	b *testing.B,
	senderCount, pendingPer, queuedPer int,
) (*ethPoolCore, map[thor.Address]uint64, ethPrepare) {
	b.Helper()

	costs := newCostTracker()
	options := Options{
		EthAccountSlots: pendingPer,
		EthAccountQueue: queuedPer,
		Limit:           senderCount * (pendingPer + queuedPer),
	}
	core := newEthPoolCore(costs, options)
	stateNonces := make(map[thor.Address]uint64, senderCount)
	origins := make(map[*TxObject]thor.Address, senderCount*(pendingPer+queuedPer))
	balance := big.NewInt(1_000_000_000)

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
				Transaction: trx,
				timeAdded:   time.Now().UnixNano(),
				executable:  nonce < pendingPer,
			}
			setTestPriorityGasPrice(txObj, big.NewInt(1))
			origins[txObj] = origin
			core.allByHash[txObj.Hash()] = txObj
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
		core.senders[origin] = sender
	}

	prepare := func(txObj *TxObject) ethPreparation {
		origin, fabricated := origins[txObj]
		if !fabricated {
			origin = txObj.Origin()
		}
		return ethPreparation{
			request: reservationRequest{
				owner:   ethReservationOwner(origin, txObj.Nonce()),
				payer:   origin,
				cost:    big.NewInt(1),
				balance: balance,
			},
			viable:           true,
			priorityGasPrice: big.NewInt(1),
		}
	}
	return core, stateNonces, prepare
}

// BenchmarkEthPoolCoreRevalidateDefaultLimit measures the per-head-change cost:
// this is the only maintenance path that reads chain state, so it is the one whose
// cadence matters.
func BenchmarkEthPoolCoreRevalidateDefaultLimit(b *testing.B) {
	const (
		senderCount = 125
		pendingPer  = 16
		queuedPer   = 64
	)
	core, stateNonces, prepare := benchmarkEthCorePool(b, senderCount, pendingPer, queuedPer)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := core.revalidate(stateNonces, pendingPer, prepare); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEthPoolCoreSweepDefaultLimit measures the per-tick cost. Sweeping reads
// no chain state, so this is the whole cost of a housekeeping tick on a pool whose
// head has not moved.
func BenchmarkEthPoolCoreSweepDefaultLimit(b *testing.B) {
	const (
		senderCount = 125
		pendingPer  = 16
		queuedPer   = 64
	)
	core, _, _ := benchmarkEthCorePool(b, senderCount, pendingPer, queuedPer)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := core.sweep(); err != nil {
			b.Fatal(err)
		}
	}
}

// ethCoreScalingSizes are the pool sizes the single-transaction mutation
// benchmarks sweep. Cost must stay flat across them: a mutation touches one
// sender, so nothing it does may be proportional to the pool.
var ethCoreScalingSizes = []int{100, 1_000, 10_000}

func BenchmarkEthPoolCoreAddScaling(b *testing.B) {
	const pendingPer = 16
	for _, poolSize := range ethCoreScalingSizes {
		b.Run(strconv.Itoa(poolSize), func(b *testing.B) {
			core, _, prepare := benchmarkEthCorePool(b, poolSize/pendingPer, pendingPer, 0)
			candidate := benchmarkEthCoreObject(b, 0)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, _, err := core.add(
					candidate, 0, 0, pendingPer, 64, 10, prepare,
				); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				core.removeByHash(candidate.Hash())
				b.StartTimer()
			}
		})
	}
}

func BenchmarkEthPoolCoreRemoveScaling(b *testing.B) {
	const pendingPer = 16
	for _, poolSize := range ethCoreScalingSizes {
		b.Run(strconv.Itoa(poolSize), func(b *testing.B) {
			core, _, prepare := benchmarkEthCorePool(b, poolSize/pendingPer, pendingPer, 0)
			candidate := benchmarkEthCoreObject(b, 0)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				if _, _, err := core.add(
					candidate, 0, 0, pendingPer, 64, 10, prepare,
				); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if !core.removeByHash(candidate.Hash()) {
					b.Fatal("candidate not removed")
				}
			}
		})
	}
}
