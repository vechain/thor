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

		merged := orderExecutableStreams(sources, total)
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
