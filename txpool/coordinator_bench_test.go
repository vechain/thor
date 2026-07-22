// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func BenchmarkMergeExecutables(b *testing.B) {
	const (
		senderCount = 64
		perSender   = 16
	)
	groups := make([][]executableTx, senderCount)
	for sender := range senderCount {
		groups[sender] = make([]executableTx, perSender)
		for nonce := range perSender {
			groups[sender][nonce] = executableTx{
				tx:               &tx.Transaction{},
				priorityGasPrice: big.NewInt(int64(senderCount - sender)),
				timeAdded:        int64(nonce),
			}
		}
	}
	eth := ethExecutablesSnapshot{groups: groups, total: senderCount * perSender}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = mergePoolExecutables(nil, eth)
	}
}

func BenchmarkCoordinatorExecutables(b *testing.B) {
	const (
		vechainCount = 1_000
		senderCount  = 64
		perSender    = 16
	)
	vechainEntries := make([]executableTx, vechainCount)
	vechainTxs := make(tx.Transactions, vechainCount)
	for i := range vechainCount {
		entry := executableTx{
			tx:               &tx.Transaction{},
			priorityGasPrice: big.NewInt(int64(vechainCount - i)),
			timeAdded:        int64(i),
		}
		vechainEntries[i] = entry
		vechainTxs[i] = entry.tx
	}
	vechain := &VeChainPool{}
	vechain.executables.Store(&vechainExecutablesSnapshot{
		transactions: vechainTxs,
		entries:      vechainEntries,
	})

	ethMap := newEthPoolMap(newCostTracker())
	for senderIndex := range senderCount {
		var origin thor.Address
		origin[0] = byte(senderIndex + 1)
		sender := newEthSender(origin, 0)
		for nonce := range perSender {
			sender.pending[uint64(nonce)] = &TxObject{
				Transaction:      &tx.Transaction{},
				priorityGasPrice: big.NewInt(int64(senderCount - senderIndex)),
				timeAdded:        int64(nonce),
				executable:       true,
			}
		}
		ethMap.senders[origin] = sender
	}
	coordinator := &TxPoolCoordinator{
		vechain: vechain,
		eth:     &EthPool{all: ethMap},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = coordinator.Executables()
	}
}

func BenchmarkCostTrackerReconcile(b *testing.B) {
	tracker := newCostTracker()
	payer := thor.Address{0x01}
	owner := ethReservationOwner(payer, 0)
	request := []reservationRequest{{
		owner:   owner,
		payer:   payer,
		cost:    big.NewInt(1),
		balance: big.NewInt(1_000_000),
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := tracker.reconcile([]reservationOwner{owner}, request, requireAllReservations); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCostTrackerReconcileParallel(b *testing.B) {
	tracker := newCostTracker()
	payer := thor.Address{0x01}
	owners := make([]reservationOwner, 256)
	requests := make([][]reservationRequest, len(owners))
	for i := range owners {
		owners[i] = ethReservationOwner(payer, uint64(i))
		requests[i] = []reservationRequest{{
			owner:   owners[i],
			payer:   payer,
			cost:    big.NewInt(1),
			balance: big.NewInt(1_000_000),
		}}
	}
	var cursor atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			index := cursor.Add(1) % uint64(len(owners))
			_, _ = tracker.reconcile(
				[]reservationOwner{owners[index]},
				requests[index],
				requireAllReservations,
			)
		}
	})
}

func BenchmarkVeChainPoolWash(b *testing.B) {
	const txCount = 1_000
	pool := newPool(txCount, txCount, &thor.NoFork)
	defer pool.Close()

	txs := make(tx.Transactions, txCount)
	for i := range txCount {
		txs[i] = newTx(
			tx.TypeLegacy,
			pool.repo.ChainTag(),
			nil,
			21_000,
			tx.BlockRef{},
			100,
			nil,
			tx.Features(0),
			devAccounts[i%len(devAccounts)],
		)
	}
	pool.Fill(txs)
	head := pool.repo.BestBlockSummary()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, _, err := pool.wash(head, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEthPoolReinjectFromFork(b *testing.B) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	if err != nil {
		b.Fatal(err)
	}
	pool := NewEth(tchain.Repo(), tchain.Stater(), Options{
		Limit:           1_000,
		MaxLifetime:     time.Hour,
		EthAccountSlots: 16,
		EthAccountQueue: 64,
		EthPriceBump:    10,
	}, &thor.SoloFork)
	defer pool.Close()

	baseFee := tchain.Repo().BestBlockSummary().Header.BaseFee()
	txs := make(tx.Transactions, 0, 64)
	for signerIndex := range 8 {
		for nonce := range 8 {
			to := devAccounts[(signerIndex+1)%len(devAccounts)].Address
			trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
				ChainID(tchain.Repo().ChainID()).
				Nonce(uint64(nonce)).
				Gas(21_000).
				MaxFeePerGas(new(big.Int).Mul(baseFee, big.NewInt(2))).
				MaxPriorityFeePerGas(big.NewInt(100)).
				To(&to).
				Build(), devAccounts[signerIndex].PrivateKey)
			txs = append(txs, trx)
		}
	}
	fork := ForkReinjection{Discarded: txs}

	b.ReportAllocs()
	for range b.N {
		b.StartTimer()
		if err := pool.ReinjectFromFork(fork); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		for _, trx := range txs {
			pool.Remove(trx.Hash(), trx.ID())
		}
	}
}
