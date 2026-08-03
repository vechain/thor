// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func FuzzMergeExecutableStreams(f *testing.F) {
	f.Add([]byte{2, 10, 1, 2, 20, 2, 5, 3})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			data = data[:128]
		}
		sources := make([][]executableTx, 0)
		sourceByTx := make(map[*tx.Transaction]int)
		total := 0
		for cursor := 0; cursor < len(data); {
			length := int(data[cursor] % 5)
			cursor++
			sourceIndex := len(sources)
			source := make([]executableTx, 0, length)
			for range length {
				if cursor+1 >= len(data) {
					break
				}
				entry := executableTx{
					tx:               &tx.Transaction{},
					priorityGasPrice: new(big.Int).SetUint64(uint64(data[cursor])),
					timeAdded:        int64(data[cursor+1]),
				}
				cursor += 2
				sourceByTx[entry.tx] = sourceIndex
				source = append(source, entry)
				total++
			}
			sources = append(sources, source)
		}

		merged := orderExecutableStreams(nil, sources)
		require.Len(t, merged, total)
		lastIndex := make(map[int]int)
		for _, trx := range merged {
			sourceIndex, ok := sourceByTx[trx]
			require.True(t, ok)
			index := -1
			for i, entry := range sources[sourceIndex] {
				if entry.tx == trx {
					index = i
					break
				}
			}
			require.Greater(t, index, lastIndex[sourceIndex]-1)
			lastIndex[sourceIndex] = index + 1
		}
	})
}

func FuzzFeeBumped(f *testing.F) {
	f.Add(uint64(100), uint64(110), uint64(10))
	f.Add(uint64(1), uint64(1), uint64(10))
	f.Add(uint64(0), uint64(1), uint64(100))

	f.Fuzz(func(t *testing.T, oldValue, candidateValue, bump uint64) {
		bump %= 1_000
		oldFee := new(big.Int).SetUint64(oldValue)
		candidate := new(big.Int).SetUint64(candidateValue)
		threshold := new(big.Int).Mul(oldFee, new(big.Int).SetUint64(100+bump))
		threshold.Div(threshold, big.NewInt(100))
		expected := candidate.Cmp(oldFee) > 0 && candidate.Cmp(threshold) >= 0

		require.Equal(t, expected, feeBumped(oldFee, candidate, bump))
	})
}

func FuzzEthSenderMutations(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 0, 3, 2, 4, 1})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 128 {
			operations = operations[:128]
		}
		sender := newEthSender(thor.Address{0x01}, 0)
		for _, operation := range operations {
			switch operation % 5 {
			case 0:
				nonce := sender.poolNonce()
				delete(sender.queue, nonce)
				sender.pending[nonce] = feeTx(10, 1)
			case 1:
				if len(sender.pending) > 0 {
					nonce := sender.stateNonce + uint64(operation)%uint64(len(sender.pending))
					sender.dropNonce(nonce)
				}
			case 2:
				if len(sender.pending) > 0 {
					nonce := sender.stateNonce + uint64(operation)%uint64(len(sender.pending)+1)
					sender.demoteFrom(nonce)
				}
			case 3:
				next := uint64(operation % 16)
				sender.syncStateNonce(next)
			case 4:
				next := uint64(operation % 16)
				sender.resetStateNonce(next)
			}

			for nonce := range sender.queue {
				require.NotContains(t, sender.pending, nonce)
			}
			for nonce := sender.stateNonce; nonce < sender.poolNonce(); nonce++ {
				require.Contains(t, sender.pending, nonce)
			}
			require.Equal(t, sender.stateNonce+uint64(len(sender.pending)), sender.poolNonce())
		}
	})
}

// FuzzEthPreparationCoverage asserts that the preparation pre-pass covers every
// transaction the commit asks about, across all four mutation paths. A non-zero
// fallback count means the pre-pass and the committer disagree about which
// transitions are possible: the commit still succeeds, but it read chain state
// while holding the core write lock.
// func FuzzEthPreparationCoverage(f *testing.F) {
// 	f.Add([]byte{0, 0, 0, 1, 0, 2, 1, 0, 2, 1, 3, 2, 4, 0})
// 	f.Add([]byte{0, 3, 0, 4, 0, 5, 2, 0, 0, 3, 1, 1})
// 	f.Add([]byte{})

// 	f.Fuzz(func(t *testing.T, script []byte) {
// 		if len(script) > 96 {
// 			script = script[:96]
// 		}
// 		const (
// 			senders      = 3
// 			globalLimit  = 64
// 			pendingLimit = 4
// 			queueLimit   = 6
// 			priceBump    = 10
// 		)

// 		options := Options{
// 			MaxLifetime:     time.Duration(argument%3) * time.Nanosecond,
// 			EthAccountSlots: pendingLimit,
// 			EthAccountQueue: queueLimit,
// 			Limit:           globalLimit,
// 		}
// 		core := newEthPoolCore(newCostTracker(), options)
// 		stateNonces := make(map[thor.Address]uint64, senders)
// 		for signer := range senders {
// 			stateNonces[devAccounts[signer].Address] = 0
// 		}

// 		// Vary viability and affordability so commits exercise break-on-unviable,
// 		// demote-then-repromote, and partial-prefix acceptance.
// 		prepare := func(txObj *TxObject) ethPreparation {
// 			if txObj.Nonce()%7 == 6 {
// 				return ethPreparation{}
// 			}
// 			return ethPreparation{
// 				request: reservationRequest{
// 					owner:   ethReservationOwner(txObj.Origin(), txObj.Nonce()),
// 					payer:   txObj.Origin(),
// 					cost:    big.NewInt(10),
// 					balance: big.NewInt(int64(20 + 10*(txObj.Nonce()%4))),
// 				},
// 				viable:           true,
// 				priorityGasPrice: big.NewInt(1),
// 			}
// 		}

// 		baseline := ethPreparationFallbacks.Load()
// 		for cursor := 0; cursor+1 < len(script); cursor += 2 {
// 			operation, argument := script[cursor], script[cursor+1]
// 			signer := int(argument) % senders
// 			origin := devAccounts[signer].Address
// 			nonce := uint64(argument) % 8

// 			switch operation % 5 {
// 			case 0:
// 				txObj := newEthCoreTestObjectWithTip(t, nonce, 10+int64(argument%4), 1+int64(argument%3), signer)
// 				_, _, _ = core.add(
// 					txObj, stateNonces[origin],
// 					globalLimit, pendingLimit, queueLimit, priceBump, prepare,
// 				)
// 			case 1:
// 				// Move one account's canonical nonce forward or backward.
// 				stateNonces[origin] = uint64(argument) % 4
// 				_, _ = core.syncHead(stateNonces, pendingLimit, prepare)
// 			case 2:
// 				_, _ = core.sweep()
// 				_, _, _ = core.revalidate(stateNonces, pendingLimit, prepare)
// 			case 3:
// 				candidate := newEthCoreTestObjectWithTip(t, nonce, 20+int64(argument%4), 2+int64(argument%3), signer)
// 				stateNonces[origin] = uint64(argument) % 3
// 				_, _ = core.reconcileFork(
// 					[]ethForkCandidate{{txObj: candidate, stateNonce: stateNonces[origin]}},
// 					stateNonces,
// 					globalLimit, pendingLimit, queueLimit, priceBump, prepare,
// 				)
// 			case 4:
// 				if sender := core.senders[origin]; sender != nil {
// 					if txObj := sender.get(nonce); txObj != nil {
// 						core.removeByHash(txObj.Hash())
// 					}
// 				}
// 			}

// 			require.Equal(t, baseline, ethPreparationFallbacks.Load(),
// 				"preparation window missed a transaction the commit needed")
// 		}
// 	})
// }

func FuzzCostTrackerReconcile(f *testing.F) {
	f.Add([]byte{0, 10, 0, 1, 20, 1, 0, 2, 30})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 192 {
			operations = operations[:192]
		}
		tracker := newCostTracker()
		payer := thor.Address{0x02}
		balance := big.NewInt(1_000)
		for cursor := 0; cursor+2 < len(operations); cursor += 3 {
			owner := ethReservationOwner(payer, uint64(operations[cursor+1]%16))
			if operations[cursor]%2 == 0 {
				before := tracker.pendingCost(payer)
				cost := new(big.Int).SetUint64(uint64(operations[cursor+2]) * 10)
				err := tracker.reserve(owner, payer, cost, balance)
				if err != nil {
					require.Equal(t, before, tracker.pendingCost(payer))
				}
			} else {
				require.NoError(t, tracker.release(owner))
			}

			pending := tracker.pendingCost(payer)
			require.GreaterOrEqual(t, pending.Sign(), 0)
			require.LessOrEqual(t, pending.Cmp(balance), 0)
			sum := new(big.Int)
			for _, reservation := range tracker.reservations {
				if reservation.payer == payer {
					sum.Add(sum, reservation.cost)
				}
			}
			require.Equal(t, sum, pending)
		}
	})
}

func FuzzBlocklistParser(f *testing.F) {
	f.Add([]byte("0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n"))
	f.Add([]byte("not-an-address"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4_096 {
			data = data[:4_096]
		}
		var bl blocklist
		list, err := bl.readList(bytes.NewReader(data))
		if err == nil {
			require.NotNil(t, list)
		}
	})
}
