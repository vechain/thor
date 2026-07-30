// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func TestCostTrackerReserveRelease(t *testing.T) {
	tr := newCostTracker()
	payer := thor.BytesToAddress([]byte{1})
	owner1 := vechainReservationOwner(thor.BytesToBytes32([]byte{1}))
	owner2 := vechainReservationOwner(thor.BytesToBytes32([]byte{2}))
	balance := big.NewInt(100)

	require.NoError(t, tr.reserve(owner1, payer, big.NewInt(60), balance))
	assert.Equal(t, big.NewInt(60), tr.pendingCost(payer))

	err := tr.reserve(owner2, payer, big.NewInt(50), balance)
	require.ErrorIs(t, err, errInsufficientEnergy)
	assert.Equal(t, big.NewInt(60), tr.pendingCost(payer), "failed reserve must not mutate")

	require.NoError(t, tr.reserve(owner2, payer, big.NewInt(40), balance))
	assert.Equal(t, big.NewInt(100), tr.pendingCost(payer))

	require.NoError(t, tr.release(owner1))
	assert.Equal(t, big.NewInt(40), tr.pendingCost(payer))

	require.NoError(t, tr.release(owner2))
	assert.Zero(t, tr.pendingCost(payer).Sign())

	require.NoError(t, tr.release(owner2))
	assert.Zero(t, tr.pendingCost(payer).Sign())
}

func TestCostTrackerConcurrentReserve(t *testing.T) {
	tr := newCostTracker()
	payer := thor.BytesToAddress([]byte{2})
	balance := big.NewInt(100)
	amount := big.NewInt(60)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)
	for i := range 2 {
		wg.Go(func() {
			owner := vechainReservationOwner(thor.BytesToBytes32([]byte{byte(i + 1)}))
			err := tr.reserve(owner, payer, amount, balance)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "only one of two overlapping reserves may succeed")
	assert.Equal(t, amount, tr.pendingCost(payer))
}

func TestCostTrackerReplaceReservation(t *testing.T) {
	tr := newCostTracker()
	payer := thor.BytesToAddress([]byte{3})
	owner := vechainReservationOwner(thor.BytesToBytes32([]byte{3}))

	require.NoError(t, tr.reserve(owner, payer, big.NewInt(60), big.NewInt(100)))
	require.NoError(t, tr.reserve(owner, payer, big.NewInt(80), big.NewInt(100)))
	assert.Equal(t, big.NewInt(80), tr.pendingCost(payer))
}

func TestCostTrackerReconcileRollback(t *testing.T) {
	tr := newCostTracker()
	payer := thor.BytesToAddress([]byte{4})
	oldOwner := vechainReservationOwner(thor.BytesToBytes32([]byte{4}))
	newOwner := vechainReservationOwner(thor.BytesToBytes32([]byte{5}))
	require.NoError(t, tr.reserve(oldOwner, payer, big.NewInt(60), big.NewInt(100)))

	accepted, err := tr.reconcile(
		[]reservationOwner{oldOwner},
		[]reservationRequest{{
			owner:   newOwner,
			payer:   payer,
			cost:    big.NewInt(110),
			balance: big.NewInt(100),
		}},
		requireAllReservations,
	)

	require.ErrorIs(t, err, errInsufficientEnergy)
	assert.Zero(t, accepted)
	assert.Equal(t, big.NewInt(60), tr.pendingCost(payer))
	require.NoError(t, tr.release(oldOwner))
	assert.Zero(t, tr.pendingCost(payer).Sign())
}

func TestCostTrackerReconcileAffordablePrefix(t *testing.T) {
	tr := newCostTracker()
	payer := thor.BytesToAddress([]byte{5})
	owner1 := ethReservationOwner(payer, 1)
	owner2 := ethReservationOwner(payer, 2)

	accepted, err := tr.reconcile(nil, []reservationRequest{
		{owner: owner1, payer: payer, cost: big.NewInt(60), balance: big.NewInt(100)},
		{owner: owner2, payer: payer, cost: big.NewInt(50), balance: big.NewInt(100)},
	}, acceptAffordablePrefix)

	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, big.NewInt(60), tr.pendingCost(payer))
	require.NoError(t, tr.release(owner1, owner2))
	assert.Zero(t, tr.pendingCost(payer).Sign())
}

func TestCostTrackerRejectsDuplicateDesiredOwnerWithoutMutation(t *testing.T) {
	tr := newCostTracker()
	payer := thor.Address{0x61}
	existing := ethReservationOwner(payer, 0)
	duplicate := ethReservationOwner(payer, 1)
	require.NoError(t, tr.reserve(existing, payer, big.NewInt(30), big.NewInt(100)))

	accepted, err := tr.reconcile(
		[]reservationOwner{existing},
		[]reservationRequest{
			{owner: duplicate, payer: payer, cost: big.NewInt(20), balance: big.NewInt(100)},
			{owner: duplicate, payer: payer, cost: big.NewInt(10), balance: big.NewInt(100)},
		},
		requireAllReservations,
	)

	require.EqualError(t, err, "cost tracker: duplicate reservation owner")
	assert.Zero(t, accepted)
	assert.Equal(t, big.NewInt(30), tr.pendingCost(payer))
	assert.Contains(t, tr.reservations, existing)
	assert.NotContains(t, tr.reservations, duplicate)
}

func TestCostTrackerZeroCostReservationLifecycle(t *testing.T) {
	tr := newCostTracker()
	payer := thor.Address{0x62}
	owner := ethReservationOwner(payer, 0)

	require.NoError(t, tr.reserve(owner, payer, new(big.Int), new(big.Int)))
	assert.NotContains(t, tr.pending, payer)
	require.Contains(t, tr.reservations, owner)
	assert.Zero(t, tr.reservations[owner].cost.Sign())

	require.NoError(t, tr.release(owner))
	assert.NotContains(t, tr.pending, payer)
	assert.NotContains(t, tr.reservations, owner)
}

func TestCostTrackerDeletesPendingEntryAtZero(t *testing.T) {
	tr := newCostTracker()
	payer := thor.Address{0x63}
	owner := vechainReservationOwner(thor.Bytes32{0x63})

	require.NoError(t, tr.reserve(owner, payer, big.NewInt(25), big.NewInt(25)))
	require.Contains(t, tr.pending, payer)
	require.NoError(t, tr.release(owner))

	assert.NotContains(t, tr.pending, payer)
	assert.Zero(t, tr.pendingCost(payer).Sign())
}

func TestCostTrackerMapAddRelease(t *testing.T) {
	costs := newCostTracker()
	m := newTxObjectMap(costs)

	acc, repo, best, st, forkConfig := SetupTest()
	trx := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	txObj, err := ResolveTx(trx, false)
	require.NoError(t, err)

	baseFee := galacticaBaseFee(best)
	ok, err := txObj.Executable(repo.NewChain(best.Header().ID()), st, best.Header(), forkConfig, baseFee)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, m.Add(txObj, 10, new(big.Int).Set(txObj.Cost())))
	assert.Equal(t, txObj.Cost(), costs.pendingCost(*txObj.Payer()))
	require.True(t, m.RemoveByHash(txObj.Hash()))
	assert.Zero(t, costs.pendingCost(*txObj.Payer()).Sign())
}

func galacticaBaseFee(best *block.Block) *big.Int {
	if bf := best.Header().BaseFee(); bf != nil {
		return bf
	}
	return big.NewInt(0)
}

func TestCostTrackerStressSharedLedger(t *testing.T) {
	costs := newCostTracker()
	payer := thor.BytesToAddress([]byte{9})
	budget := big.NewInt(1000)

	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			for i := range 50 {
				owner := vechainReservationOwner(thor.BytesToBytes32([]byte{byte(worker), byte(i)}))
				amount := big.NewInt(10)
				err := costs.reserve(owner, payer, amount, budget)
				if err == nil {
					require.NoError(t, costs.release(owner))
				}
			}
		})
	}
	wg.Wait()
	assert.Zero(t, costs.pendingCost(payer).Sign())
}

func limitedEnergyPoolFixture(t *testing.T) (*chain.Repository, *state.Stater, *thor.ForkConfig) {
	t.Helper()

	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		ForkConfig(&thor.NoFork).
		Timestamp(now).
		State(func(st *state.State) error {
			if err := st.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := st.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}
			bal, _ := new(big.Int).SetString("42000000000000000000", 10)
			for _, acc := range devAccounts {
				st.SetEnergy(acc.Address, bal, now)
			}
			return nil
		})

	method, found := builtin.Params.ABI.MethodByName("set")
	require.True(t, found)

	var executor thor.Address
	data, err := method.EncodeInput(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	require.NoError(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data, err = method.EncodeInput(thor.KeyLegacyTxBaseGasPrice, thor.InitialBaseGasPrice)
	require.NoError(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	b0, _, _, err := builder.Build(state.NewStater(db))
	require.NoError(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	require.NoError(t, err)
	root, err := stage.Commit()
	require.NoError(t, err)

	var feat tx.Features
	feat.SetDelegated(true)
	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		GasLimit(thor.InitialGasLimit).
		TransactionFeatures(feat).Build()

	forkConfig := thor.NoFork
	forkConfig.VIP191 = 0

	repo, err := chain.NewRepository(db, b0)
	require.NoError(t, err)
	require.NoError(t, repo.AddBlock(b1, tx.Receipts{}, 0, true))

	return repo, state.NewStater(db), &forkConfig
}

func TestCostTrackerCrossPoolAdmission(t *testing.T) {
	repo, stater, forkConfig := limitedEnergyPoolFixture(t)
	costs := newCostTracker()
	opts := Options{Limit: LIMIT, LimitPerAccount: LIMIT, MaxLifetime: time.Hour}

	poolA := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolA.Close()
	poolB := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolB.Close()

	acc := devAccounts[0]
	require.NoError(t, poolA.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))
	require.NoError(t, poolA.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))

	err := poolB.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc))
	require.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	assert.Equal(t, 2, poolA.Len())
	assert.Equal(t, 0, poolB.Len())
}

func TestCostTrackerCrossPoolAdmissionReverseOrder(t *testing.T) {
	repo, stater, forkConfig := limitedEnergyPoolFixture(t)
	costs := newCostTracker()
	opts := Options{Limit: LIMIT, LimitPerAccount: LIMIT, MaxLifetime: time.Hour}

	poolA := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolA.Close()
	poolB := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolB.Close()

	acc := devAccounts[0]
	require.NoError(t, poolB.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))
	require.NoError(t, poolB.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))

	err := poolA.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc))
	require.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
}

func TestCostTrackerReleaseAllowsSiblingPoolAdmission(t *testing.T) {
	repo, stater, forkConfig := limitedEnergyPoolFixture(t)
	costs := newCostTracker()
	opts := Options{Limit: LIMIT, LimitPerAccount: LIMIT, MaxLifetime: time.Hour}

	poolA := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolA.Close()
	poolB := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolB.Close()

	acc := devAccounts[0]
	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	tx2 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	tx3 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	require.NoError(t, poolA.AddRemote(tx1))
	require.NoError(t, poolA.AddRemote(tx2))
	require.EqualError(t, poolB.AddRemote(tx3), "tx rejected: insufficient energy for overall pending cost")

	require.True(t, poolA.Remove(tx1.Hash(), tx1.ID()))
	require.NoError(t, poolB.AddRemote(tx3))
	assert.NotNil(t, poolB.Get(tx3.ID()))
}

func TestEthRemovalReleasesCostForVeChainAdmission(t *testing.T) {
	repo, stater, forkConfig := limitedEnergyPoolFixture(t)
	costs := newCostTracker()
	opts := Options{Limit: LIMIT, LimitPerAccount: LIMIT, MaxLifetime: time.Hour}
	vechainPool := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer vechainPool.Close()

	ethCore := newEthPoolCore(costs, opts)
	ethTxObj := newEthCoreTestObject(t, 0, 10, 0)
	origin := ethTxObj.Origin()
	ethTxObj.executable = true
	sender := newEthSender(origin, 0)
	sender.pending[0] = ethTxObj
	ethCore.senders[origin] = sender
	ethCore.allByHash[ethTxObj.Hash()] = ethTxObj

	balance, ok := new(big.Int).SetString("42000000000000000000", 10)
	require.True(t, ok)
	require.NoError(t, costs.reserve(
		ethReservationOwner(origin, 0),
		origin,
		balance,
		balance,
	))

	nativeTx := newTx(
		tx.TypeLegacy,
		repo.ChainTag(),
		nil,
		21000,
		tx.BlockRef{},
		100,
		nil,
		tx.Features(0),
		devAccounts[0],
	)
	require.EqualError(t, vechainPool.AddRemote(nativeTx), "tx rejected: insufficient energy for overall pending cost")

	ethPool := &EthPool{core: ethCore}
	require.True(t, ethPool.Remove(ethTxObj.Hash(), ethTxObj.ID()))
	require.NoError(t, vechainPool.AddRemote(nativeTx))
	assert.NotNil(t, vechainPool.Get(nativeTx.ID()))
}

func TestCostTrackerWashPromotionRespectsSiblingReservation(t *testing.T) {
	repo, stater, forkConfig := limitedEnergyPoolFixture(t)
	costs := newCostTracker()
	opts := Options{Limit: LIMIT, LimitPerAccount: LIMIT, MaxLifetime: time.Hour}

	poolA := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolA.Close()
	poolB := newVeChainPool(repo, stater, opts, forkConfig, costs, new(blocklist))
	defer poolB.Close()

	acc := devAccounts[0]
	require.NoError(t, poolA.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))
	require.NoError(t, poolA.AddRemote(newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)))

	trx := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
	txObj, err := ResolveTx(trx, false)
	require.NoError(t, err)
	head := repo.BestBlockSummary()
	baseFee := poolB.baseFeeCache.Get(head.Header)
	exec, err := txObj.Executable(repo.NewChain(head.Header.ID()), stater.NewState(head.Root()), head.Header, forkConfig, baseFee)
	require.NoError(t, err)
	require.True(t, exec)
	txObj.executable = false
	poolB.all.Fill([]*TxObject{txObj})

	executables, _, _, err := poolB.wash(head, false)
	require.NoError(t, err)
	for _, e := range executables {
		assert.NotEqual(t, trx.ID(), e.ID(), "tx must not promote when sibling pool holds the budget")
	}
	assert.Nil(t, poolB.Get(trx.ID()), "unaffordable promotion must wash the tx out")
}

func TestReserveCost(t *testing.T) {
	payer := thor.BytesToAddress([]byte{0x71})

	tests := []struct {
		name         string
		setupPending *big.Int
		cost         *big.Int
		balance      *big.Int
		wantErr      error
		wantPending  *big.Int
		wantAbsent   bool
	}{
		{
			name:        "first reservation within balance",
			cost:        big.NewInt(60),
			balance:     big.NewInt(100),
			wantPending: big.NewInt(60),
		},
		{
			name:         "accumulates existing pending",
			setupPending: big.NewInt(30),
			cost:         big.NewInt(20),
			balance:      big.NewInt(100),
			wantPending:  big.NewInt(50),
		},
		{
			name:         "exact balance succeeds",
			setupPending: big.NewInt(40),
			cost:         big.NewInt(60),
			balance:      big.NewInt(100),
			wantPending:  big.NewInt(100),
		},
		{
			name:         "zero cost preserves existing pending",
			setupPending: big.NewInt(40),
			cost:         new(big.Int),
			balance:      big.NewInt(100),
			wantPending:  big.NewInt(40),
		},
		{
			name:       "zero cost on empty pending is no-op",
			cost:       new(big.Int),
			balance:    big.NewInt(100),
			wantAbsent: true,
		},
		{
			name:         "insufficient energy rejects",
			setupPending: big.NewInt(60),
			cost:         big.NewInt(50),
			balance:      big.NewInt(100),
			wantErr:      errInsufficientEnergy,
			wantPending:  big.NewInt(60),
		},
		{
			name:       "insufficient energy on first reservation",
			cost:       big.NewInt(101),
			balance:    big.NewInt(100),
			wantErr:    errInsufficientEnergy,
			wantAbsent: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newCostTracker()
			if tc.setupPending != nil {
				tr.pending[payer] = new(big.Int).Set(tc.setupPending)
			}

			err := tr.reserveCost(payer, tc.cost, tc.balance)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantAbsent {
				assert.NotContains(t, tr.pending, payer)
				return
			}
			assert.Equal(t, tc.wantPending, tr.pending[payer])
		})
	}
}

func TestReleaseCost(t *testing.T) {
	payer := thor.BytesToAddress([]byte{0x72})

	tests := []struct {
		name         string
		setupPending *big.Int
		cost         *big.Int
		wantPending  *big.Int
		wantAbsent   bool
	}{
		{
			name:         "zero cost is no-op",
			setupPending: big.NewInt(50),
			cost:         new(big.Int),
			wantPending:  big.NewInt(50),
		},
		{
			name:         "full release deletes pending entry",
			setupPending: big.NewInt(100),
			cost:         big.NewInt(100),
			wantAbsent:   true,
		},
		{
			name:         "partial release subtracts cost",
			setupPending: big.NewInt(100),
			cost:         big.NewInt(40),
			wantPending:  big.NewInt(60),
		},
		{
			name:         "negative cost increases pending",
			setupPending: big.NewInt(100),
			cost:         big.NewInt(-30),
			wantPending:  big.NewInt(130),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newCostTracker()
			tr.pending[payer] = new(big.Int).Set(tc.setupPending)

			tr.releaseCost(payer, tc.cost)

			if tc.wantAbsent {
				assert.NotContains(t, tr.pending, payer)
				return
			}
			assert.Equal(t, tc.wantPending, tr.pending[payer])
		})
	}
}

func TestAddCost(t *testing.T) {
	payer := thor.BytesToAddress([]byte{0x73})
	other := thor.BytesToAddress([]byte{0x74})

	tests := []struct {
		name  string
		setup map[thor.Address]*big.Int
		run   func(costs map[thor.Address]*big.Int)
		want  map[thor.Address]*big.Int
	}{
		{
			name:  "zero cost is no-op",
			setup: make(map[thor.Address]*big.Int),
			run: func(costs map[thor.Address]*big.Int) {
				addCost(costs, payer, new(big.Int))
			},
			want: map[thor.Address]*big.Int{},
		},
		{
			name:  "creates entry for new payer",
			setup: make(map[thor.Address]*big.Int),
			run: func(costs map[thor.Address]*big.Int) {
				addCost(costs, payer, big.NewInt(25))
			},
			want: map[thor.Address]*big.Int{payer: big.NewInt(25)},
		},
		{
			name:  "accumulates existing entry",
			setup: map[thor.Address]*big.Int{payer: big.NewInt(25)},
			run: func(costs map[thor.Address]*big.Int) {
				addCost(costs, payer, big.NewInt(10))
			},
			want: map[thor.Address]*big.Int{payer: big.NewInt(35)},
		},
		{
			name:  "tracks payers independently",
			setup: make(map[thor.Address]*big.Int),
			run: func(costs map[thor.Address]*big.Int) {
				addCost(costs, payer, big.NewInt(25))
				addCost(costs, other, big.NewInt(40))
			},
			want: map[thor.Address]*big.Int{
				payer: big.NewInt(25),
				other: big.NewInt(40),
			},
		},
		{
			name:  "negative cost subtracts from existing entry",
			setup: map[thor.Address]*big.Int{payer: big.NewInt(25)},
			run: func(costs map[thor.Address]*big.Int) {
				addCost(costs, payer, big.NewInt(-10))
			},
			want: map[thor.Address]*big.Int{payer: big.NewInt(15)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			costs := tc.setup
			tc.run(costs)
			assert.Equal(t, tc.want, costs)
		})
	}
}

func TestCostTrackerRollback(t *testing.T) {
	payer := thor.BytesToAddress([]byte{0x75})
	oldOwner := vechainReservationOwner(thor.Bytes32{0x75})
	newOwner1 := vechainReservationOwner(thor.Bytes32{0x76})
	newOwner2 := vechainReservationOwner(thor.Bytes32{0x77})

	tests := []struct {
		name             string
		setup            func(tr *costTracker)
		old              map[reservationOwner]reservation
		accepted         []reservationRequest
		wantPending      *big.Int
		wantReservations []reservationOwner
		wantAbsent       []reservationOwner
	}{
		{
			name: "restores prior reservations and pending",
			setup: func(tr *costTracker) {
				tr.pending[payer] = big.NewInt(30)
				tr.reservations[newOwner1] = reservation{payer: payer, cost: big.NewInt(30)}
			},
			old: map[reservationOwner]reservation{
				oldOwner: {payer: payer, cost: big.NewInt(60)},
			},
			accepted: []reservationRequest{{
				owner:   newOwner1,
				payer:   payer,
				cost:    big.NewInt(30),
				balance: big.NewInt(100),
			}},
			wantPending:      big.NewInt(60),
			wantReservations: []reservationOwner{oldOwner},
			wantAbsent:       []reservationOwner{newOwner1},
		},
		{
			name: "rolls back multiple accepted reservations",
			setup: func(tr *costTracker) {
				tr.pending[payer] = big.NewInt(90)
				tr.reservations[newOwner1] = reservation{payer: payer, cost: big.NewInt(50)}
				tr.reservations[newOwner2] = reservation{payer: payer, cost: big.NewInt(40)}
			},
			old: map[reservationOwner]reservation{
				oldOwner: {payer: payer, cost: big.NewInt(60)},
			},
			accepted: []reservationRequest{
				{owner: newOwner1, payer: payer, cost: big.NewInt(50), balance: big.NewInt(200)},
				{owner: newOwner2, payer: payer, cost: big.NewInt(40), balance: big.NewInt(200)},
			},
			wantPending:      big.NewInt(60),
			wantReservations: []reservationOwner{oldOwner},
			wantAbsent:       []reservationOwner{newOwner1, newOwner2},
		},
		{
			name: "empty rollback is no-op",
			setup: func(tr *costTracker) {
				tr.pending[payer] = big.NewInt(60)
				tr.reservations[oldOwner] = reservation{payer: payer, cost: big.NewInt(60)}
			},
			wantPending:      big.NewInt(60),
			wantReservations: []reservationOwner{oldOwner},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newCostTracker()
			if tc.setup != nil {
				tc.setup(tr)
			}

			tr.rollback(tc.old, tc.accepted)

			assert.Equal(t, tc.wantPending, tr.pending[payer])
			for _, owner := range tc.wantReservations {
				assert.Contains(t, tr.reservations, owner)
			}
			for _, owner := range tc.wantAbsent {
				assert.NotContains(t, tr.reservations, owner)
			}
		})
	}
}

func TestCostTrackerPendingCost(t *testing.T) {
	payer := thor.BytesToAddress([]byte{0x76})

	tests := []struct {
		name         string
		setupPending *big.Int
		want         *big.Int
		mutateCopy   bool
	}{
		{
			name: "returns zero for unknown payer",
			want: new(big.Int),
		},
		{
			name:         "returns copy of pending amount",
			setupPending: big.NewInt(42),
			want:         big.NewInt(42),
			mutateCopy:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newCostTracker()
			if tc.setupPending != nil {
				tr.pending[payer] = new(big.Int).Set(tc.setupPending)
			}

			got := tr.pendingCost(payer)
			assert.Equal(t, tc.want, got)

			if tc.mutateCopy {
				got.Add(got, big.NewInt(1))
				assert.Equal(t, tc.setupPending, tr.pending[payer], "caller mutation must not affect tracker state")
			}
		})
	}
}
