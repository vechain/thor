// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ceiling used by every case below: chain head 10M raised by 1/1024 four times.
// 10000000 -> 10009765 -> 10019540 -> 10029324 -> 10039118
const testCeiling = 10_039_118

func newPrevalidateConsensus(t *testing.T, forkConfig *thor.ForkConfig) *Consensus {
	db := muxdb.NewMem()
	gen, _ := genesis.NewDevnet()
	stater := state.NewStater(db)

	genesisBlock, _, _, err := gen.Build(stater)
	assert.NoError(t, err)

	repo, err := chain.NewRepository(db, genesisBlock)
	assert.NoError(t, err)

	return New(repo, stater, forkConfig)
}

// simpleTx builds a signed single-clause tx, then optionally overwrites the
// signature with `sigLen` zero bytes to simulate padding.
func simpleTx(t *testing.T, chainTag byte, sigLen int) *tx.Transaction {
	trx := tx.NewBuilder(tx.TypeLegacy).ChainTag(chainTag).Gas(21000).Expiration(10).Build()
	trx = tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
	if sigLen > 0 {
		trx = trx.WithSignature(make([]byte, sigLen))
	}
	return trx
}

// lowGasTx builds a signed tx whose declared Gas is below its intrinsic gas
// (21000 for a single, 0-clause tx).
func lowGasTx(t *testing.T, chainTag byte) *tx.Transaction {
	trx := tx.NewBuilder(tx.TypeLegacy).ChainTag(chainTag).Gas(1).Expiration(10).Build()
	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}

// txWithUnusedReserved returns a signed tx whose reserved field carries one
// unused slot — legit signing never produces this (tx/reserved.go only caps
// the slot count, not each slot's bytes), so it's crafted directly at the
// RLP level: patch the Reserved item of the encoded legacy tx to
// [Features(0), one opaque slot] and decode it back.
func txWithUnusedReserved(t *testing.T, chainTag byte) *tx.Transaction {
	raw, err := simpleTx(t, chainTag, 0).MarshalBinary()
	assert.NoError(t, err)

	var fields []rlp.RawValue
	assert.NoError(t, rlp.DecodeBytes(raw, &fields))

	reservedRaw, err := rlp.EncodeToBytes([]rlp.RawValue{{0x80}, {0x01}})
	assert.NoError(t, err)
	fields[8] = reservedRaw // Reserved is the 9th legacy tx field.

	patched, err := rlp.EncodeToBytes(fields)
	assert.NoError(t, err)

	out := new(tx.Transaction)
	assert.NoError(t, out.UnmarshalBinary(patched))
	return out
}

// scenario pins the header signature/alpha shape a valid block must have on
// either side of VIP214, so the same table below exercises both — including
// the mainnet-active VIP214 branch, not just pre-fork.
type scenario struct {
	name       string
	forkConfig *thor.ForkConfig
	sig        []byte // valid header signature for this fork
	alpha      []byte // valid alpha for this fork; nil pre-VIP214
}

func TestPrevalidateStateless(t *testing.T) {
	scenarios := []scenario{
		{name: "pre-VIP214", forkConfig: &thor.NoFork, sig: make([]byte, 65)},
		{name: "VIP214 active", forkConfig: &thor.ForkConfig{VIP214: 0}, sig: make([]byte, block.ComplexSigSize), alpha: make([]byte, 32)},
	}

	for _, scn := range scenarios {
		t.Run(scn.name, func(t *testing.T) {
			c := newPrevalidateConsensus(t, scn.forkConfig)
			tag := c.repo.ChainTag()

			// 1904 txs at 21000 intrinsic gas each is 39,984,000 — over a 10M limit.
			manyTxs := func(n int) []*tx.Transaction {
				out := make([]*tx.Transaction, 0, n)
				for range n {
					out = append(out, simpleTx(t, tag, 0))
				}
				return out
			}

			build := func(gasLimit uint64, sig []byte, txs []*tx.Transaction) *block.Block {
				b := new(block.Builder).GasLimit(gasLimit)
				if scn.alpha != nil {
					b = b.Alpha(scn.alpha)
				}
				for _, trx := range txs {
					b = b.Transaction(trx)
				}
				blk := b.Build()
				if sig != nil {
					blk = blk.WithSignature(sig)
				}
				return blk
			}

			tests := []struct {
				name      string
				blk       *block.Block
				expectErr bool
			}{
				{
					name:      "normal block passes",
					blk:       build(10_000_000, scn.sig, manyTxs(10)),
					expectErr: false,
				},
				{
					name:      "gas limit exactly at ceiling passes",
					blk:       build(testCeiling, scn.sig, nil),
					expectErr: false,
				},
				{
					name:      "gas limit one above ceiling rejected",
					blk:       build(testCeiling+1, scn.sig, nil),
					expectErr: true,
				},
				{
					// Without the ceiling check this block would pass: a maxed gas limit
					// makes the intrinsic gas sum vacuously true.
					name:      "max gas limit rejected",
					blk:       build(^uint64(0), scn.sig, manyTxs(10)),
					expectErr: true,
				},
				{
					// 500 txs * 21000 = 10.5M > 10M limit.
					name:      "intrinsic gas sum over limit rejected",
					blk:       build(10_000_000, scn.sig, manyTxs(500)),
					expectErr: true,
				},
				{
					name:      "padded tx signature rejected",
					blk:       build(10_000_000, scn.sig, []*tx.Transaction{simpleTx(t, tag, 4096)}),
					expectErr: true,
				},
				{
					name:      "tx gas below intrinsic rejected",
					blk:       build(10_000_000, scn.sig, []*tx.Transaction{lowGasTx(t, tag)}),
					expectErr: true,
				},
				{
					name:      "wrong chain tag rejected",
					blk:       build(10_000_000, scn.sig, []*tx.Transaction{simpleTx(t, tag+1, 0)}),
					expectErr: true,
				},
				{
					name:      "wrong header signature length rejected",
					blk:       build(10_000_000, make([]byte, 4096), nil),
					expectErr: true,
				},
				{
					// IntrinsicGas never prices reserved fields, so this passes the
					// gas-sum check and must be caught separately.
					name:      "tx with unused reserved field rejected",
					blk:       build(10_000_000, scn.sig, []*tx.Transaction{txWithUnusedReserved(t, tag)}),
					expectErr: true,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					err := c.PrevalidateStateless(tt.blk, testCeiling)
					if tt.expectErr {
						assert.Error(t, err)
					} else {
						assert.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestPrevalidateStatelessVIP214HeaderSignature(t *testing.T) {
	c := newPrevalidateConsensus(t, &thor.ForkConfig{VIP214: 0})

	withAlpha := new(block.Builder).GasLimit(10_000_000).Alpha(make([]byte, 32)).Build()
	noAlpha := new(block.Builder).GasLimit(10_000_000).Build()
	oversizedAlpha := new(block.Builder).GasLimit(10_000_000).Alpha(make([]byte, 4096)).Build()

	assert.Error(t, c.PrevalidateStateless(withAlpha.WithSignature(make([]byte, 65)), testCeiling),
		"65-byte header signature must be rejected after VIP214")
	assert.NoError(t, c.PrevalidateStateless(withAlpha.WithSignature(make([]byte, block.ComplexSigSize)), testCeiling),
		"ComplexSigSize header signature with 32-byte alpha must be accepted after VIP214")
	assert.Error(t, c.PrevalidateStateless(noAlpha.WithSignature(make([]byte, block.ComplexSigSize)), testCeiling),
		"empty alpha must be rejected after VIP214")
	assert.Error(t, c.PrevalidateStateless(oversizedAlpha.WithSignature(make([]byte, block.ComplexSigSize)), testCeiling),
		"oversized alpha must be rejected after VIP214")
}
