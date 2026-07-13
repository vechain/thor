// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func newChainRepo() *chain.Repository {
	tchain, _ := testchain.NewWithFork(&thor.SoloFork, 180)
	return tchain.Repo()
}

func newTx(
	txType tx.Type,
	chainTag byte,
	clauses []*tx.Clause,
	gas uint64,
	blockRef tx.BlockRef,
	expiration uint32,
	dependsOn *thor.Bytes32,
	features tx.Features,
	from genesis.DevAccount,
) *tx.Transaction {
	trx := txBuilder(txType, chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build()
	return tx.MustSign(trx, from.PrivateKey)
}

func newDelegatedTx(
	txType tx.Type,
	chainTag byte,
	clauses []*tx.Clause,
	gas uint64,
	blockRef tx.BlockRef,
	expiration uint32,
	dependsOn *thor.Bytes32,
	from genesis.DevAccount,
	delegator genesis.DevAccount,
) *tx.Transaction {
	var features tx.Features
	features.SetDelegated(true)

	return tx.MustSignDelegated(
		txBuilder(txType, chainTag, clauses, gas, blockRef, expiration, dependsOn, features).Build(),
		from.PrivateKey,
		delegator.PrivateKey,
	)
}

// mineLegacyTxWithWork builds and signs a legacy tx whose UnprovedWork() is at least
// minWork. UnprovedWork() = MaxBig256/hash(txFields, nonce) is unpredictable from the
// fields alone, so this brute-forces the nonce until the threshold is met. hashWithoutNonce
// (and therefore the search) does not depend on the nonce field, so a single unsigned
// builder can be reused across candidate nonces.
func mineLegacyTxWithWork(
	t *testing.T,
	chainTag byte,
	blockRef tx.BlockRef,
	expiration uint32,
	from genesis.DevAccount,
	minWork *big.Int,
) *tx.Transaction {
	t.Helper()

	builder := txBuilder(tx.TypeLegacy, chainTag, nil, 21000, blockRef, expiration, nil, tx.Features(0))
	evalWork := builder.Build().EvaluateWork(from.Address)

	const maxAttempts = 20_000_000
	for nonce := uint64(0); nonce < maxAttempts; nonce++ {
		if evalWork(nonce).Cmp(minWork) >= 0 {
			return tx.MustSign(builder.Nonce(nonce).Build(), from.PrivateKey)
		}
	}
	t.Fatalf("failed to mine a legacy tx nonce with work >= %v within %d attempts", minWork, maxAttempts)
	return nil
}

func txBuilder(
	txType tx.Type,
	chainTag byte,
	clauses []*tx.Clause,
	gas uint64,
	blockRef tx.BlockRef,
	expiration uint32,
	dependsOn *thor.Bytes32,
	features tx.Features,
) *tx.Builder {
	builder := tx.NewBuilder(txType).ChainTag(chainTag)
	for _, c := range clauses {
		builder.Clause(c)
	}

	return builder.BlockRef(blockRef).
		Expiration(expiration).
		Nonce(rand.Uint64()). //#nosec G404
		DependsOn(dependsOn).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee)).
		MaxPriorityFeePerGas(big.NewInt(10000)).
		Features(features).
		Gas(gas)
}

func SetupTest() (genesis.DevAccount, *chain.Repository, *block.Block, *state.State, *thor.ForkConfig) {
	tchain, _ := testchain.NewWithFork(&thor.SoloFork, 180)
	repo := tchain.Repo()
	db := tchain.Database()

	b0 := repo.GenesisBlock()
	b1 := new(block.Builder).ParentID(b0.Header().ID()).GasLimit(10000000).TotalScore(100).BaseFee(big.NewInt(thor.InitialBaseFee)).Build()
	repo.AddBlock(b1, nil, 0, false)
	st := state.New(db, trie.Root{Hash: repo.GenesisBlock().Header().StateRoot()})

	return genesis.DevAccounts()[0], tchain.Repo(), b1, st, tchain.GetForkConfig()
}

func TestExecutableWithError(t *testing.T) {
	acc, repo, b1, st, fc := SetupTest()

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), acc), false, ""},
	}

	baseFee := galactica.CalcBaseFee(b1.Header(), fc)
	for _, tt := range tests {
		txObj, err := ResolveTx(tt.tx, false)
		assert.Nil(t, err)

		// pass custom headID
		chain := repo.NewChain(thor.Bytes32{0})

		exe, _, err := txObj.Evaluate(chain, st, b1.Header(), fc, baseFee, false)
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Equal(t, err.Error(), "leveldb: not found")
			assert.Equal(t, tt.expected, exe)
		}
	}
}

func newTestTxObj(priorityGasPrice *big.Int, timeAdded int64, payer *thor.Address) *TxObject {
	o := &TxObject{timeAdded: timeAdded}
	o.setPricing(&txPricing{priorityGasPrice: priorityGasPrice, payer: payer})
	return o
}

func TestSort(t *testing.T) {
	addr1 := thor.BytesToAddress([]byte("addr1"))
	addr2 := thor.BytesToAddress([]byte("addr2"))
	objs := []*TxObject{
		newTestTxObj(big.NewInt(0), 0, nil),
		newTestTxObj(big.NewInt(10), 20, &addr1),
		newTestTxObj(big.NewInt(10), 3, &addr2),
		newTestTxObj(big.NewInt(20), 0, nil),
		newTestTxObj(big.NewInt(30), 0, nil),
	}
	sortTxObjsByPriorityGasPriceDesc(objs)

	assert.Equal(t, big.NewInt(30), objs[0].priorityGasPrice())
	assert.Equal(t, big.NewInt(20), objs[1].priorityGasPrice())
	assert.Equal(t, big.NewInt(10), objs[2].priorityGasPrice())
	assert.Equal(t, int64(20), objs[2].timeAdded)
	assert.Equal(t, &addr1, objs[2].Payer())
	assert.Equal(t, big.NewInt(10), objs[3].priorityGasPrice())
	assert.Equal(t, int64(3), objs[3].timeAdded)
	assert.Equal(t, &addr2, objs[3].Payer())
	assert.Equal(t, big.NewInt(0), objs[4].priorityGasPrice())
}

func TestResolve(t *testing.T) {
	acc := genesis.DevAccounts()[0]
	trx := newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	txObj, err := ResolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)

	assert.Equal(t, acc.Address, txObj.Origin())

	trx = newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	txObj, err = ResolveTx(trx, false)
	assert.Nil(t, err)
	assert.Equal(t, trx, txObj.Transaction)
	assert.Equal(t, acc.Address, txObj.Origin())
}

func TestExecutable(t *testing.T) {
	acc := genesis.DevAccounts()[0]

	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	assert.Nil(t, err)
	repo := tchain.Repo()
	db := tchain.Database()

	b0 := repo.GenesisBlock()
	st := state.New(db, trie.Root{Hash: repo.GenesisBlock().Header().StateRoot()})

	tests := []struct {
		tx          *tx.Transaction
		expected    bool
		expectedErr string
	}{
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeLegacy, 0, nil, b0.Header().GasLimit(), tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeLegacy, 0, nil, b0.Header().GasLimit()+1, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "tx gas exceeds block gas limit"},
		{newTx(tx.TypeLegacy, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "tx gas exceeds block gas limit"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), true, ""},
		{newTx(tx.TypeDynamicFee, 0, nil, math.MaxUint64, tx.BlockRef{}, 100, nil, tx.Features(0), acc), false, "tx gas exceeds block gas limit"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{1}, 100, nil, tx.Features(0), acc), true, "block ref out of schedule"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 0, nil, tx.Features(0), acc), true, "expired"},
		{newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, &thor.Bytes32{}, tx.Features(0), acc), false, ""},
	}

	baseFee := galactica.CalcBaseFee(b0.Header(), tchain.GetForkConfig())
	for _, tt := range tests {
		txObj, err := ResolveTx(tt.tx, false)
		assert.Nil(t, err)

		exe, _, err := txObj.Evaluate(repo.NewChain(b0.Header().ID()), st, b0.Header(), tchain.GetForkConfig(), baseFee, false)
		if tt.expectedErr != "" {
			assert.Equal(t, tt.expectedErr, err.Error())
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, exe)
		}
	}
}

func TestExecutableRejectNonLegacyBeforeGalactica(t *testing.T) {
	forkConfig := &thor.ForkConfig{
		GALACTICA: 2,
		HAYABUSA:  math.MaxUint32,
	}
	hayabusaTP := uint32(math.MaxUint32)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	dynamicFeeTx := newTx(tx.TypeDynamicFee, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])

	tchain, _ := testchain.NewWithFork(forkConfig, 180)
	repo := tchain.Repo()
	baseFee := galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig)

	txObj1, err := ResolveTx(dynamicFeeTx, false)
	assert.Nil(t, err)

	st := tchain.Stater().NewState(repo.BestBlockSummary().Root())
	exe, _, err := txObj1.Evaluate(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee, false)
	assert.False(t, exe)
	assert.Equal(t, tx.ErrTxTypeNotSupported, err)

	legacyTx := newTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	txObj2, err := ResolveTx(legacyTx, false)
	assert.Nil(t, err)

	exe, _, err = txObj2.Evaluate(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee, false)
	assert.True(t, exe)
	assert.Nil(t, err)

	// add a block 1
	tchain.MintBlock()

	// recalculate the base fee since new block is added
	baseFee = galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig)
	st = tchain.Stater().NewState(repo.BestBlockSummary().Root())
	_, _, err = txObj1.Evaluate(repo.NewBestChain(), st, repo.BestBlockSummary().Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
}

func TestEvaluateAndPricingSnapshot(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	assert.Nil(t, err)
	tchain.MintBlock()
	repo, stater, forkConfig := tchain.Repo(), tchain.Stater(), tchain.GetForkConfig()

	trx := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	txObj, err := ResolveTx(trx, false)
	assert.Nil(t, err)

	best := repo.BestBlockSummary()
	state := stater.NewState(best.Root())
	baseFee := galactica.CalcBaseFee(best.Header, forkConfig)

	executable, pricing, err := txObj.Evaluate(repo.NewBestChain(), state, best.Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
	assert.True(t, executable)
	assert.NotNil(t, pricing)

	// Evaluate did not publish, so accessors still return nil
	assert.Nil(t, txObj.Cost())
	assert.Nil(t, txObj.Payer())
	assert.Nil(t, txObj.priorityGasPrice())

	// after publishing, accessors read the snapshot
	txObj.setPricing(pricing)
	assert.Equal(t, pricing.cost, txObj.Cost())
	assert.Equal(t, pricing.payer, txObj.Payer())
	assert.Equal(t, pricing.priorityGasPrice, txObj.priorityGasPrice())
}

func TestExecutableRejectUnsupportedFeatures(t *testing.T) {
	forkConfig := &thor.ForkConfig{
		VIP191:   2,
		HAYABUSA: math.MaxUint32,
	}
	hayabusaTP := uint32(math.MaxUint32)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	tchain, _ := testchain.NewWithFork(forkConfig, 180)
	repo := tchain.Repo()

	tx1 := newDelegatedTx(tx.TypeLegacy, 0, nil, 21000, tx.BlockRef{0}, 100, nil, genesis.DevAccounts()[0], genesis.DevAccounts()[1])
	txObj1, err := ResolveTx(tx1, false)
	assert.Nil(t, err)

	st := tchain.Stater().NewState(repo.BestBlockSummary().Root())
	exe, _, err := txObj1.Evaluate(
		repo.NewBestChain(),
		st,
		repo.BestBlockSummary().Header,
		forkConfig,
		galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig),
		false,
	)
	assert.False(t, exe)
	assert.ErrorContains(t, err, "unsupported features")

	// add a block 1
	tchain.MintBlock()

	st = tchain.Stater().NewState(repo.BestBlockSummary().Root())
	_, _, err = txObj1.Evaluate(
		repo.NewBestChain(),
		st,
		repo.BestBlockSummary().Header,
		forkConfig,
		galactica.CalcBaseFee(repo.BestBlockSummary().Header, forkConfig),
		false,
	)
	assert.Nil(t, err)
}

func TestBaseFeeRefreshMatchesCanonical(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	assert.Nil(t, err)
	tchain.MintBlock()
	repo, stater, forkConfig := tchain.Repo(), tchain.Stater(), tchain.GetForkConfig()
	best := repo.BestBlockSummary()
	state := stater.NewState(best.Root())
	chain := repo.NewBestChain()

	// legacyTxBaseGasPrice must match the value Evaluate bakes into the cached legacy
	// ceiling (the on-chain governance param), otherwise the oracle would be fed a
	// different lbgp than the cache and legacy comparisons would be meaningless.
	lbgp, err := builtin.Params.Native(state).Get(thor.KeyLegacyTxBaseGasPrice)
	assert.Nil(t, err)
	nextBlockNum := best.Header.Number() + 1

	for _, txType := range []tx.Type{tx.TypeLegacy, tx.TypeDynamicFee} {
		// For legacy txs, use a block ref that prefix-matches a real on-chain block
		// (the best block itself) so ProvedWork returns a nonzero UnprovedWork() within
		// the work window, exercising the work-included ceiling rather than always
		// comparing no-work to no-work (see tx.TestProvedWork for the ProvedWork
		// prefix-match semantics this relies on).
		var trx *tx.Transaction
		if txType == tx.TypeLegacy {
			blockRef := tx.NewBlockRefFromID(best.Header.ID())
			// A raw nonzero UnprovedWork() is not enough: workToGas() divides by
			// workPerGas(=1000) and floors, so a "nonzero" work below that threshold
			// still yields wgas=0 and collapses back to the no-work ceiling. Mine a
			// nonce with comfortably large work so the work-included branch of
			// OverallGasPrice actually kicks in.
			trx = mineLegacyTxWithWork(t, repo.ChainTag(), blockRef, 100, genesis.DevAccounts()[0], big.NewInt(50000))
		} else {
			trx = newTx(txType, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
		}
		txObj, err := ResolveTx(trx, false)
		assert.Nil(t, err)

		baseFee0 := galactica.CalcBaseFee(best.Header, forkConfig)
		_, pricing, err := txObj.Evaluate(chain, state, best.Header, forkConfig, baseFee0, false)
		assert.Nil(t, err)
		assert.NotNil(t, pricing)
		txObj.setPricing(pricing)

		provedWork, err := txObj.ProvedWork(nextBlockNum, chain.GetBlockID)
		assert.Nil(t, err)
		if txType == tx.TypeLegacy {
			assert.Equal(t, 1, provedWork.Sign(), "legacy provedWork must be nonzero to exercise the work-included ceiling")
		}

		// At multiple baseFees, the refresh result must equal the canonical
		// EffectivePriorityFeePerGas computed with the (possibly nonzero) provedWork.
		for _, bf := range []*big.Int{big.NewInt(0), big.NewInt(1), new(big.Int).Set(baseFee0), new(big.Int).Mul(baseFee0, big.NewInt(3))} {
			txObj.refreshPriorityGasPrice(bf, lbgp, nextBlockNum)
			want := txObj.EffectivePriorityFeePerGas(bf, lbgp, provedWork)
			assert.Equal(t, 0, want.Cmp(txObj.priorityGasPrice()), "type=%v baseFee=%v", txType, bf)
		}

		// Capture the non-expired (work-included, for legacy) ceiling at baseFee0.
		txObj.refreshPriorityGasPrice(baseFee0, lbgp, nextBlockNum)
		nonExpiredPGP := new(big.Int).Set(txObj.priorityGasPrice())
		wantNonExpired := txObj.EffectivePriorityFeePerGas(baseFee0, lbgp, provedWork)
		assert.Equal(t, 0, wantNonExpired.Cmp(nonExpiredPGP), "type=%v non-expired", txType)

		// Once the proved work expires (head advances past refNum+MaxTxWorkDelay), the
		// legacy ceiling must drop to the no-work overall gas price.
		expiredBlockNum := txObj.BlockRef().Number() + thor.MaxTxWorkDelay + 1
		txObj.refreshPriorityGasPrice(baseFee0, lbgp, expiredBlockNum)
		expiredPGP := txObj.priorityGasPrice()
		wantExpired := txObj.EffectivePriorityFeePerGas(baseFee0, lbgp, new(big.Int)) // provedWork=0
		assert.Equal(t, 0, wantExpired.Cmp(expiredPGP), "type=%v expired", txType)

		if txType == tx.TypeLegacy {
			// The work-included and no-work ceilings must actually differ, proving the
			// work->no-work transition (not no-work->no-work) is what's being observed.
			assert.NotEqual(t, 0, nonExpiredPGP.Cmp(expiredPGP), "type=%v non-expired and expired legacy pgp must differ", txType)
		}
	}
}
