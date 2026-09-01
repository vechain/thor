// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// SELFDESTRUCT behaviour surface. "settle" means advancing both accounts'
// energy settlement point (SetEnergy at the current block time); the account
// itself is deleted by the opcode after the callback returns.
//
//	regime                          toSelf  settle*  move bal  ERC20 log  tx.Transfer  account
//	------------------------------  ------  -------  --------  ---------  -----------  --------
//	pre-EIP-6780                    no      yes      yes       energy!=0  bal!=0       deleted
//	  (opSuicide)                   yes     no       no        energy!=0  bal!=0       deleted
//	EIP-6780, created in clause     no      yes      yes       energy!=0  bal!=0       deleted
//	  (shouldDestroy=true)          yes     no       no        energy!=0  bal!=0       deleted
//	EIP-6780, pre-existing          no      yes      yes       energy!=0  bal!=0       survives
//	  (shouldDestroy=false)         yes     no       no        no         no           survives
//
//	* settle runs when bal!=0 || energy!=0
//
// The first two regimes are identical by design: whenever the account ends up
// deleted, the output must match the pre-EIP-6780 one. The last row is the only
// genuinely new cell, and it is a complete no-op.
//
// Two non-obvious points:
//   - energy!=0 together with shouldDestroy=true is reachable: the contract
//     address is deterministic, so VTHO can be credited to it before creation.
//   - toSelf plus deletion burns both VET and VTHO while the events still
//     describe a transfer. This matches pre-EIP-6780 and is invisible to
//     Energy.TotalSupply/TotalBurned, since state.Delete bypasses Energy.Add/Sub.

const secondsInYear = 365 * 24 * 3600

// TestSelfDestructTransferToReceiver checks that a SELFDESTRUCT transfer to a
// pre-existing receiver also advances the receiver's energy settlement point
// to the transfer's block time, across both opcode variants and with growth
// already stopped.
func TestSelfDestructTransferToReceiver(t *testing.T) {
	cases := []struct {
		name string
		// thor.NoFork keeps the growth window open: the devnet default (SoloFork)
		// has HAYABUSA=0, which stops energy growth already at genesis.
		genesisFork   thor.ForkConfig
		genesisStaker bool // whether the genesis needs a PoS staker instead of an authority node
		runtimeFork   thor.ForkConfig
		growthEnabled bool
	}{
		{
			name:          "legacy SELFDESTRUCT with energy growth active",
			genesisFork:   thor.NoFork,
			runtimeFork:   thor.NoFork,
			growthEnabled: true,
		},
		{
			name:          "EIP-6780 SELFDESTRUCT with energy growth active",
			genesisFork:   thor.NoFork,
			runtimeFork:   eip6780RuntimeFork(),
			growthEnabled: true,
		},
		{
			name:          "energy growth stopped at genesis",
			genesisFork:   growthStoppedAtGenesisFork(),
			genesisStaker: true,
			runtimeFork:   thor.NoFork,
			growthEnabled: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runSelfDestructTransferCase(t, c.genesisFork, c.genesisStaker, c.runtimeFork, c.growthEnabled)
		})
	}
}

// eip6780RuntimeFork activates INTERSTELLAR at block 0, which maps to
// vm.ChainConfig.OsakaBlock (see runtime.New) and selects the osaka
// instruction set, whose SELFDESTRUCT is opSuicide6780 instead of opSuicide.
func eip6780RuntimeFork() thor.ForkConfig {
	fc := thor.NoFork
	fc.INTERSTELLAR = 0
	return fc
}

// growthStoppedAtGenesisFork activates HAYABUSA at block 0, so runtime.New
// (invoked during genesis block construction) calls
// builtin.Energy.StopEnergyGrowth() with the genesis timestamp, freezing
// growth for every account from t0. genesis.NewDevnetWithConfig also requires
// a PoS-flavored genesis (Stakers instead of Authority, HayabusaTP == 0) once
// HAYABUSA == 0, see the genesisStaker flag in runSelfDestructTransferCase.
func growthStoppedAtGenesisFork() thor.ForkConfig {
	fc := thor.NoFork
	fc.HAYABUSA = 0
	return fc
}

func runSelfDestructTransferCase(t *testing.T, genesisFork thor.ForkConfig, genesisStaker bool, runtimeFork thor.ForkConfig, growthEnabled bool) {
	db := muxdb.NewMem()
	devConfig := genesis.DevConfig{ForkConfig: &genesisFork}
	if genesisStaker {
		devConfig.Config = &genesis.SoloConfig
	}
	g := genesis.NewDevnetWithConfig(devConfig)
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address

	receiver := thor.BytesToAddress([]byte("selfdestruct-receiver"))
	oldBalance := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18)) // 1,000 VET
	t0 := b0.Header().Timestamp()

	// Settle R's account at t0: non-zero balance + settlement point == t0,
	// so any later CalcEnergy call has a well-defined growth window to check.
	assert.Nil(t, st.SetBalance(receiver, oldBalance))
	assert.Nil(t, st.SetEnergy(receiver, big.NewInt(0), t0))

	now := t0 + 3*secondsInYear

	drained := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18)) // 500,000 VET

	// initcode: PUSH20 <receiver>; SELFDESTRUCT
	initcode := append([]byte{0x73}, receiver.Bytes()...)
	initcode = append(initcode, 0xff)

	clause := tx.NewClause(nil).WithValue(drained).WithData(initcode)

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &runtimeFork)
	exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)
	if !assert.NotNil(t, out.ContractAddress, "clause is a contract creation, ContractAddress must be set") {
		return
	}
	selfDestructed := *out.ContractAddress

	// (a) VET balances: the transfer landed fully on R, nothing left on the
	// self-destructed contract.
	receiverBalance, err := st.GetBalance(receiver)
	assert.Nil(t, err)
	wantReceiverBalance := new(big.Int).Add(oldBalance, drained)
	assert.Zerof(t, receiverBalance.Cmp(wantReceiverBalance),
		"receiver VET balance mismatch: expected=%s actual=%s", wantReceiverBalance, receiverBalance)

	contractBalance, err := st.GetBalance(selfDestructed)
	assert.Nil(t, err)
	assert.Zerof(t, contractBalance.Sign(), "self-destructed contract should hold no VET, actual=%s", contractBalance)

	// (b) VTHO balance at `now`: only oldBalance sat in R's account during the
	// [t0, now) window, so growth (if any) must be computed off oldBalance,
	// not off the post-transfer balance.
	wantEnergyAtNow := big.NewInt(0)
	if growthEnabled {
		wantEnergyAtNow = energyGrowth(oldBalance, now-t0)
	}
	energyAtNow, err := builtin.Energy.Native(st, now).Get(receiver)
	assert.Nil(t, err)
	assert.Zerof(t, energyAtNow.Cmp(wantEnergyAtNow),
		"receiver VTHO balance at transfer time mismatch: expected=%s actual=%s", wantEnergyAtNow, energyAtNow)

	// (c) Settlement point: read directly from the account trie (see
	// settlementPointOf).
	settlementPoint := settlementPointOf(t, db, st, receiver)
	assert.Equalf(t, now, settlementPoint,
		"receiver settlement point mismatch: got=%d want=%d (t0=%d)", settlementPoint, now, t0)

	if !growthEnabled {
		later := now + secondsInYear
		energyAtLater, err := builtin.Energy.Native(st, later).Get(receiver)
		assert.Nil(t, err)
		assert.Zerof(t, energyAtNow.Sign(), "receiver VTHO should stay 0 once growth is stopped, actual=%s", energyAtNow)
		assert.Zerof(t, new(big.Int).Sub(energyAtLater, energyAtNow).Sign(),
			"receiver VTHO should not grow once growth is stopped: at now=%s at now+1y=%s", energyAtNow, energyAtLater)
	}
}

// energyGrowth computes balance * EnergyGrowthRate * seconds / 1e18.
func energyGrowth(balance *big.Int, seconds uint64) *big.Int {
	e := new(big.Int).Mul(balance, thor.EnergyGrowthRate)
	e.Mul(e, new(big.Int).SetUint64(seconds))
	e.Div(e, big.NewInt(1e18))
	return e
}

// settlementPointOf reads an account's energy settlement point
// (state.Account.BlockTime) directly from the account trie, mirroring
// state.State's own loadAccount (state/account.go): Stage/Commit persists
// st's pending changes to db under ver, then the account is looked up by its
// secure key (blake2b(addr), see state/account.go's secureKey) and decoded.
// A missing/empty trie entry means the account is empty, whose zero-value
// BlockTime is 0.
//
// Commit writes to db, so callers must not still need st to accumulate
// further changes into the same version afterwards.
func settlementPointOf(t *testing.T, db *muxdb.MuxDB, st *state.State, addr thor.Address) uint64 {
	t.Helper()

	ver := trie.Version{Major: 1}
	stage, err := st.Stage(ver)
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	tr := db.NewTrie(muxdb.AccountTrieName, trie.Root{Hash: root, Ver: ver})
	data, _, err := tr.Get(thor.Blake2b(addr[:]).Bytes())
	assert.Nil(t, err)
	if len(data) == 0 {
		return 0
	}

	var acc state.Account
	assert.Nil(t, rlp.DecodeBytes(data, &acc))
	return acc.BlockTime
}

// TestSelfDestructTransferToNeverSettledReceiver checks the same invariant as
// TestSelfDestructTransferToReceiver -- the settlement point advances to the
// transfer's block time -- but starting from a receiver whose energy has
// never been settled (state.Account.BlockTime == 0), not a stale one.
func TestSelfDestructTransferToNeverSettledReceiver(t *testing.T) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &noFork)

	// Clause 0: create C1 with Value == 0, so runtime.go's Transfer hook
	// (amount.Sign() == 0 early return) never calls SetEnergy on C1 -- its
	// settlement point stays at the zero value of a freshly created account.
	// The initcode deploys 1 byte of runtime code (not empty code) so the
	// account gets a non-empty CodeHash and counts as existing per
	// state.Account.IsEmpty.
	//
	// initcode: PUSH1 0x00; PUSH1 0x00; MSTORE8; PUSH1 0x01; PUSH1 0x00; RETURN
	deployInitcode := []byte{0x60, 0x00, 0x60, 0x00, 0x53, 0x60, 0x01, 0x60, 0x00, 0xf3}
	clause0 := tx.NewClause(nil).WithValue(big.NewInt(0)).WithData(deployInitcode)
	exec0, _ := rt.PrepareClause(clause0, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out0, _, err := exec0()
	assert.Nil(t, err)
	assert.Nil(t, out0.VMErr)
	if !assert.NotNil(t, out0.ContractAddress) {
		return
	}
	c1 := *out0.ContractAddress

	exists, err := st.Exists(c1)
	assert.Nil(t, err)
	assert.True(t, exists, "C1 should exist as a deployed account before it receives any VET")

	// Clause 1: create C2 funded with `drained` VET whose initcode
	// immediately SELFDESTRUCTs, sending its balance to C1.
	drained := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18)) // 500,000 VET
	initcode := append([]byte{0x73}, c1.Bytes()...)
	initcode = append(initcode, 0xff)
	clause1 := tx.NewClause(nil).WithValue(drained).WithData(initcode)
	exec1, _ := rt.PrepareClause(clause1, 1, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out1, _, err := exec1()
	assert.Nil(t, err)
	assert.Nil(t, out1.VMErr)

	c1Balance, err := st.GetBalance(c1)
	assert.Nil(t, err)
	assert.Zerof(t, c1Balance.Cmp(drained), "C1 VET balance mismatch: expected=%s actual=%s", drained, c1Balance)

	settlementPoint := settlementPointOf(t, db, st, c1)
	assert.Equalf(t, now, settlementPoint,
		"C1 settlement point mismatch: got=%d want=%d (t0=%d)", settlementPoint, now, t0)
}

// TestSelfDestructToSelf records the current behavior of a contract that
// self-destructs to itself, across both SELFDESTRUCT opcode variants.
//
// OnSuicideContract (runtime.go) skips both transfer branches when the
// receiver is the destructing contract itself (toSelf), but opSuicide /
// opSuicide6780 still call StateDB.Suicide regardless (opSuicide
// unconditionally; opSuicide6780 because the contract was created and
// destroyed within the same clause). Suicide deletes the account via
// state.State.Delete, zeroing balance, energy, and code hash.
func TestSelfDestructToSelf(t *testing.T) {
	cases := []struct {
		name        string
		runtimeFork thor.ForkConfig
	}{
		{name: "legacy SELFDESTRUCT", runtimeFork: thor.NoFork},
		{name: "EIP-6780 SELFDESTRUCT", runtimeFork: eip6780RuntimeFork()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := muxdb.NewMem()
			noFork := thor.NoFork
			g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
			b0, _, _, err := g.Build(state.NewStater(db))
			assert.Nil(t, err)
			repo, _ := chain.NewRepository(db, b0)
			st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

			origin := genesis.DevAccounts()[0].Address
			t0 := b0.Header().Timestamp()
			now := t0 + 3*secondsInYear

			drained := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18)) // 500,000 VET

			// initcode: ADDRESS; SELFDESTRUCT
			initcode := []byte{0x30, 0xff}
			clause := tx.NewClause(nil).WithValue(drained).WithData(initcode)

			rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &c.runtimeFork)
			exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
			out, _, err := exec()
			assert.Nil(t, err)
			assert.Nil(t, out.VMErr)
			if !assert.NotNil(t, out.ContractAddress) {
				return
			}
			contractAddr := *out.ContractAddress

			balance, err := st.GetBalance(contractAddr)
			assert.Nil(t, err)
			energy, err := builtin.Energy.Native(st, now).Get(contractAddr)
			assert.Nil(t, err)
			exists, err := st.Exists(contractAddr)
			assert.Nil(t, err)

			t.Logf("self-destruct-to-self observed state: balance=%s energy=%s exists=%v", balance, energy, exists)

			// Delete zeroes the account: VET credited at creation is not
			// preserved anywhere once it destructs to itself.
			assert.Zerof(t, balance.Sign(), "self-destruct-to-self balance: expected=0 actual=%s", balance)
			assert.Zerof(t, energy.Sign(), "self-destruct-to-self energy: expected=0 actual=%s", energy)
			assert.False(t, exists, "self-destruct-to-self account should no longer exist after Delete")
		})
	}
}

// TestSelfDestructTruthTable exercises OnSuicideContract's four boolean
// dimensions -- toSelf, shouldDestroy, contract balance != 0, contract
// energy != 0 -- and checks both state (receiver balance/energy/settlement
// point, contract balance/energy/existence/code) and the two emitted side
// channels (ERC20 Transfer log, tx.Transfer) for every reachable
// combination. See OnSuicideContract in runtime.go for the branches being
// exercised.
//
// All 16 combinations are reachable. In particular, shouldDestroy == true
// (contract created and destroyed within the same clause) does NOT force
// energy to 0: OnSuicideContract reads energy as
// builtin.Energy.Native(...).Get(contractAddr), i.e. stored + growth, and
// while growth is indeed always 0 here (no time elapses between creation
// and self-destruct), `stored` can be non-zero if the about-to-be-created
// address already held VTHO before CREATE ran -- CreateContract only checks
// code/nonce for collisions, not balance. runSelfDestructTruthTableCase
// exploits this: it pre-funds the deterministic CREATE address (see its
// comment) with energy before running the creation clause.
//
// The only quadrant not fully enumerated is toSelf && !shouldDestroy: it
// hits the "return" branch in OnSuicideContract unconditionally, regardless
// of bal/energy -- nothing is transferred, no event is emitted, and the
// contract's own bal/energy are left exactly as they were. All 4
// combinations in that quadrant take the identical code path, so only one
// representative case (bal!=0, energy!=0, to demonstrate funds/energy
// getting stuck) is tested instead of all 4.
//
// That leaves 4 (toSelf=false, shouldDestroy=false) + 4 (toSelf=false,
// shouldDestroy=true) + 4 (toSelf=true, shouldDestroy=true) + 1
// (toSelf=true, shouldDestroy=false, representative) = 13 cases below.
func TestSelfDestructTruthTable(t *testing.T) {
	cases := []struct {
		name           string
		toSelf         bool
		createInClause bool // shouldDestroy
		bal            *big.Int
		energy         *big.Int
	}{
		// toSelf=false, shouldDestroy=false (pre-existing contract, receiver != contract): all 4 combinations are distinct and reachable.
		{name: "receiver, pre-existing, bal=0 energy=0", toSelf: false, createInClause: false, bal: big.NewInt(0), energy: big.NewInt(0)},
		{name: "receiver, pre-existing, bal=0 energy!=0", toSelf: false, createInClause: false, bal: big.NewInt(0), energy: big.NewInt(500)},
		// energy==0 && bal!=0 && !toSelf: settle() still runs because the OR
		// condition is on bal.Sign()!=0, so the receiver's settlement point is
		// advanced to `now` even though the energy amount added is 0 -- no ERC20
		// Transfer log is emitted for it (that's gated on energy.Sign()!=0
		// separately).
		{name: "receiver, pre-existing, bal!=0 energy=0", toSelf: false, createInClause: false, bal: big.NewInt(1e15), energy: big.NewInt(0)},
		{name: "receiver, pre-existing, bal!=0 energy!=0", toSelf: false, createInClause: false, bal: big.NewInt(1e15), energy: big.NewInt(500)},

		// toSelf=false, shouldDestroy=true (created and destroyed within the clause): all 4 combinations
		// are reachable, see runSelfDestructTruthTableCase for how energy!=0 is constructed here.
		{name: "receiver, created-in-clause, bal=0 energy=0", toSelf: false, createInClause: true, bal: big.NewInt(0), energy: big.NewInt(0)},
		{name: "receiver, created-in-clause, bal!=0 energy=0", toSelf: false, createInClause: true, bal: big.NewInt(1e15), energy: big.NewInt(0)},
		{name: "receiver, created-in-clause, bal=0 energy!=0", toSelf: false, createInClause: true, bal: big.NewInt(0), energy: big.NewInt(500)},
		{name: "receiver, created-in-clause, bal!=0 energy!=0", toSelf: false, createInClause: true, bal: big.NewInt(1e15), energy: big.NewInt(500)},

		// toSelf=true, shouldDestroy=true (created and destroyed to self within the clause).
		// OnSuicideContract emits a tx.Transfer and/or an ERC20 Transfer log with
		// Sender == Recipient == contractAddr and Amount == bal/energy, describing a
		// transfer that never actually happened (the !toSelf branch that moves
		// balance/energy never runs for this call). This is the CURRENT
		// behavior; both are consensus-visible (they land in the receipt),
		// so changing them changes the receipt root.
		{name: "self, created-in-clause, bal=0 energy=0", toSelf: true, createInClause: true, bal: big.NewInt(0), energy: big.NewInt(0)},
		{
			name:   "self, created-in-clause, bal!=0 energy=0 (phantom VET transfer)",
			toSelf: true, createInClause: true, bal: big.NewInt(1e15), energy: big.NewInt(0),
		},
		{
			name:   "self, created-in-clause, bal=0 energy!=0 (phantom VTHO transfer)",
			toSelf: true, createInClause: true, bal: big.NewInt(0), energy: big.NewInt(500),
		},
		{
			name:   "self, created-in-clause, bal!=0 energy!=0 (phantom transfers)",
			toSelf: true, createInClause: true, bal: big.NewInt(1e15), energy: big.NewInt(500),
		},

		// toSelf=true, shouldDestroy=false: single representative of the collapsed quadrant (see func comment).
		{
			name:   "self, pre-existing, bal!=0 energy!=0 (representative, no-op)",
			toSelf: true, createInClause: false, bal: big.NewInt(1e15), energy: big.NewInt(500),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runSelfDestructTruthTableCase(t, c.toSelf, c.createInClause, c.bal, c.energy)
		})
	}
}

func runSelfDestructTruthTableCase(t *testing.T, toSelf, createInClause bool, bal, energy *big.Int) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	receiver := thor.BytesToAddress([]byte("selfdestruct-truthtable-receiver"))
	oldReceiverBalance := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18)) // R0, settled at t0
	if !toSelf {
		assert.Nil(t, st.SetBalance(receiver, oldReceiverBalance))
		assert.Nil(t, st.SetEnergy(receiver, big.NewInt(0), t0))
	}

	// selfdestructCode ignores its argument when toSelf: ADDRESS SELFDESTRUCT.
	// Otherwise: PUSH20 <target> SELFDESTRUCT. Valid both as initcode (contract
	// creation clause) and as pre-set runtime code (executed unconditionally
	// regardless of calldata, see the cross-clause case in runtime_test.go).
	selfdestructCode := func(target thor.Address) []byte {
		if toSelf {
			return []byte{0x30, 0xff} // ADDRESS; SELFDESTRUCT
		}
		code := append([]byte{0x73}, target.Bytes()...) // PUSH20 <target>
		return append(code, 0xff)                       // SELFDESTRUCT
	}

	eip6780Fork := eip6780RuntimeFork()
	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &eip6780Fork)

	var contractAddr thor.Address
	var out *runtime.Output
	if createInClause {
		if energy.Sign() != 0 {
			// The CREATE address is deterministic here: NewContractAddress derives
			// it from (txCtx.ID, clauseIndex, creation counter), and this test
			// always uses the zero-value ID, clause 0, and a fresh EVM (counter
			// starts at 0). Pre-funding that address with energy before the clause
			// runs gives the about-to-be-created contract a non-zero `stored`
			// energy component -- CreateContract only rejects collisions on
			// code/nonce, not balance, so it still counts as newly created.
			predicted := thor.CreateContractAddress(thor.Bytes32{}, 0, 0)
			assert.Nil(t, st.SetEnergy(predicted, energy, now))
		}
		initcode := selfdestructCode(receiver)
		clause := tx.NewClause(nil).WithValue(bal).WithData(initcode)
		exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
		out, _, err = exec()
		assert.Nil(t, err)
		assert.Nil(t, out.VMErr)
		if !assert.NotNil(t, out.ContractAddress, "clause is a contract creation, ContractAddress must be set") {
			return
		}
		contractAddr = *out.ContractAddress
	} else {
		if toSelf {
			contractAddr = thor.BytesToAddress([]byte("selfdestruct-truthtable-toself"))
		} else {
			contractAddr = thor.BytesToAddress([]byte("selfdestruct-truthtable-preexisting"))
		}
		code := selfdestructCode(receiver)
		assert.Nil(t, st.SetCode(contractAddr, code))
		assert.Nil(t, st.SetBalance(contractAddr, bal))
		// Settle the contract's own energy at `now`, not t0: OnSuicideContract
		// reads the contract's energy via CalcEnergy (growth included), and with
		// bal != 0 growth since t0 would silently add to the raw `energy` value
		// this case is trying to pin, confounding the bal/energy dimensions.
		assert.Nil(t, st.SetEnergy(contractAddr, energy, now))

		clause := tx.NewClause(&contractAddr)
		exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
		out, _, err = exec()
		assert.Nil(t, err)
		assert.Nil(t, out.VMErr)
	}

	contractBal, err := st.GetBalance(contractAddr)
	assert.Nil(t, err)
	contractEnergy, err := builtin.Energy.Native(st, now).Get(contractAddr)
	assert.Nil(t, err)
	contractExists, err := st.Exists(contractAddr)
	assert.Nil(t, err)

	settleRan := bal.Sign() != 0 || energy.Sign() != 0

	switch {
	case toSelf && !createInClause:
		// early "return" branch (runtime.go: "if toSelf && !shouldDestroy: return"):
		// nothing changes at all. The contract's own settlement point was
		// pinned to `now` at setup, so CalcEnergy's short-circuit
		// (query time <= BlockTime) makes this an exact, growth-free check.
		assert.Zerof(t, contractBal.Cmp(bal), "contract balance: expected=%s actual=%s", bal, contractBal)
		assert.Zerof(t, contractEnergy.Cmp(energy), "contract energy: expected=%s actual=%s", energy, contractEnergy)
		assert.True(t, contractExists, "toSelf && !shouldDestroy: contract must survive untouched")
		code, err := st.GetCode(contractAddr)
		assert.Nil(t, err)
		assert.NotEmpty(t, code, "toSelf && !shouldDestroy: contract code must persist")
		assert.Empty(t, out.Events, "toSelf && !shouldDestroy: no event should be emitted")
		assert.Empty(t, out.Transfers, "toSelf && !shouldDestroy: no transfer should be recorded")

	case toSelf && createInClause:
		assert.Zerof(t, contractBal.Sign(), "self-destruct-to-self balance: expected=0 actual=%s", contractBal)
		assert.Zerof(t, contractEnergy.Sign(), "self-destruct-to-self energy: expected=0 actual=%s", contractEnergy)
		assert.False(t, contractExists, "toSelf && shouldDestroy: contract must be destroyed")

		// createInClause always emits the contract's own Prototype $Master event
		// on creation (unrelated to OnSuicideContract). energy!=0 adds a phantom
		// ERC20 Transfer log on top: OnSuicideContract's early-return guard is
		// "toSelf && !shouldDestroy", so a toSelf destroy (shouldDestroy==true)
		// still falls through to the unconditional log reporting below it, with
		// contractAddr as both sender and recipient -- same phantom pattern as
		// the VET transfer below, see case comment in TestSelfDestructTruthTable.
		wantEventCount := 1
		if energy.Sign() != 0 {
			wantEventCount++
			event, _ := builtin.Energy.ABI.EventByName("Transfer")
			data, err := event.Encode(energy)
			assert.Nil(t, err)
			wantEvent := &tx.Event{
				Address: builtin.Energy.Address,
				Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(contractAddr.Bytes()), thor.BytesToBytes32(contractAddr.Bytes())},
				Data:    data,
			}
			assert.Equal(t, wantEvent, out.Events[len(out.Events)-1])
		}
		assert.Equal(t, wantEventCount, len(out.Events), "want $Master plus a phantom energy Transfer log iff energy!=0")

		if bal.Sign() != 0 {
			// out.Transfers[0] is the clause's own value-transfer hook crediting
			// `bal` to the contract at creation (unrelated to OnSuicideContract).
			// out.Transfers[1] is the phantom transfer: sender == recipient ==
			// contractAddr, describing a self-transfer of `bal` VET that never
			// moved anywhere -- current behavior, see case comment in
			// TestSelfDestructTruthTable.
			assert.Equal(t, 2, len(out.Transfers))
			assert.Equal(t, &tx.Transfer{Sender: contractAddr, Recipient: contractAddr, Amount: bal}, out.Transfers[len(out.Transfers)-1])
		} else {
			assert.Empty(t, out.Transfers)
		}

	default: // !toSelf, both shouldDestroy values
		assert.Zerof(t, contractBal.Sign(), "contract balance must be fully drained: actual=%s", contractBal)
		assert.Zerof(t, contractEnergy.Sign(), "contract energy must be fully drained: actual=%s", contractEnergy)
		if createInClause {
			assert.False(t, contractExists, "shouldDestroy=true: contract must be destroyed")
		} else {
			assert.True(t, contractExists, "shouldDestroy=false: contract must survive")
			code, err := st.GetCode(contractAddr)
			assert.Nil(t, err)
			assert.NotEmpty(t, code, "shouldDestroy=false: contract code must persist")
		}

		wantReceiverBalance := new(big.Int).Add(oldReceiverBalance, bal)
		receiverBalance, err := st.GetBalance(receiver)
		assert.Nil(t, err)
		assert.Zerof(t, receiverBalance.Cmp(wantReceiverBalance), "receiver balance: expected=%s actual=%s", wantReceiverBalance, receiverBalance)

		wantEnergyAtNow := energyGrowth(oldReceiverBalance, now-t0)
		wantEnergyAtNow.Add(wantEnergyAtNow, energy)
		receiverEnergyAtNow, err := builtin.Energy.Native(st, now).Get(receiver)
		assert.Nil(t, err)
		assert.Zerof(t, receiverEnergyAtNow.Cmp(wantEnergyAtNow), "receiver energy at now: expected=%s actual=%s", wantEnergyAtNow, receiverEnergyAtNow)

		settlementPoint := settlementPointOf(t, db, st, receiver)
		wantSettlementPoint := t0
		if settleRan {
			wantSettlementPoint = now
		}
		assert.Equalf(t, wantSettlementPoint, settlementPoint, "receiver settlement point mismatch: got=%d want=%d", settlementPoint, wantSettlementPoint)

		// creation noise (see the $Master event note in the toSelf &&
		// createInClause case above), plus a value-transfer hook crediting
		// `bal` at creation when bal != 0 -- distinct from OnSuicideContract's
		// own tx.Transfer add below.
		wantEventCount := 0
		if createInClause {
			wantEventCount++ // $Master
		}
		if energy.Sign() != 0 {
			wantEventCount++
			event, _ := builtin.Energy.ABI.EventByName("Transfer")
			data, err := event.Encode(energy)
			assert.Nil(t, err)
			wantEvent := &tx.Event{
				Address: builtin.Energy.Address,
				Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(contractAddr.Bytes()), thor.BytesToBytes32(receiver.Bytes())},
				Data:    data,
			}
			assert.Equal(t, wantEvent, out.Events[len(out.Events)-1])
		}
		assert.Equal(t, wantEventCount, len(out.Events), "energy==0: no ERC20 Transfer log should be emitted, even if settle() ran")

		wantTransferCount := 0
		if createInClause && bal.Sign() != 0 {
			wantTransferCount++ // creation-time endowment transfer
		}
		if bal.Sign() != 0 {
			wantTransferCount++
			wantTransfer := &tx.Transfer{Sender: contractAddr, Recipient: receiver, Amount: bal}
			assert.Equal(t, wantTransfer, out.Transfers[len(out.Transfers)-1])
		}
		assert.Equal(t, wantTransferCount, len(out.Transfers))
	}
}

// selfDestructEventSnapshot mirrors tx.Event but with Data left as []byte
// (already comparable) -- no big.Int fields, so assert.Equal is safe as-is.
type selfDestructEventSnapshot struct {
	Address thor.Address
	Topics  []thor.Bytes32
	Data    []byte
}

// selfDestructTransferSnapshot mirrors tx.Transfer but with Amount rendered
// as a decimal string: big.Int's zero value has two internal
// representations (abs:nil vs abs:{}) that reflect.DeepEqual (and therefore
// assert.Equal) treats as unequal, so amounts are normalized to strings
// before being placed in a struct that gets compared wholesale. (Other
// balance/energy assertions in this file sidestep the same issue by using
// Cmp instead of a struct-level comparison.)
type selfDestructTransferSnapshot struct {
	Sender    thor.Address
	Recipient thor.Address
	Amount    string
}

// selfDestructDestroyPathSnapshot captures everything a caller of
// OnSuicideContract can observe for a shouldDestroy==true invocation: the
// two emitted side channels (events, transfers) plus the resulting state of
// both the destroyed contract and (when the transfer target is not the
// contract itself) the receiver. All *big.Int-valued state is rendered as a
// decimal string for the same reason as selfDestructTransferSnapshot.Amount.
type selfDestructDestroyPathSnapshot struct {
	Events    []selfDestructEventSnapshot
	Transfers []selfDestructTransferSnapshot

	ContractBalance string
	ContractEnergy  string
	ContractExists  bool
	ContractCode    []byte

	// Populated only when toSelf == false: a toSelf destruction leaves no
	// separate receiver account to inspect (the destroyed account and the
	// would-be receiver are the same address), so these stay at their zero
	// values on both sides of the diff, which trivially compare equal.
	ReceiverBalance         string
	ReceiverEnergy          string
	ReceiverSettlementPoint uint64
}

func snapshotSelfDestructEvents(events []*tx.Event) []selfDestructEventSnapshot {
	out := make([]selfDestructEventSnapshot, len(events))
	for i, e := range events {
		out[i] = selfDestructEventSnapshot{Address: e.Address, Topics: e.Topics, Data: e.Data}
	}
	return out
}

func snapshotSelfDestructTransfers(transfers []*tx.Transfer) []selfDestructTransferSnapshot {
	out := make([]selfDestructTransferSnapshot, len(transfers))
	for i, tr := range transfers {
		out[i] = selfDestructTransferSnapshot{Sender: tr.Sender, Recipient: tr.Recipient, Amount: tr.Amount.String()}
	}
	return out
}

// runSelfDestructDestroyPathCase runs one shouldDestroy==true SELFDESTRUCT
// (the contract is created and destroyed within the same clause, so
// IsNewContract is true and both opSuicide and opSuicide6780 call Suicide
// unconditionally) under the given runtime fork and returns a snapshot of
// everything OnSuicideContract can affect, for later comparison across
// forks. See TestSelfDestructDestroyPathMatchesLegacy for why this only
// covers shouldDestroy==true.
func runSelfDestructDestroyPathCase(t *testing.T, fork thor.ForkConfig, toSelf bool, bal, energy *big.Int) selfDestructDestroyPathSnapshot {
	t.Helper()

	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	receiver := thor.BytesToAddress([]byte("selfdestruct-destroy-path-receiver"))
	oldReceiverBalance := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	if !toSelf {
		assert.Nil(t, st.SetBalance(receiver, oldReceiverBalance))
		assert.Nil(t, st.SetEnergy(receiver, big.NewInt(0), t0))
	}

	if energy.Sign() != 0 {
		// Same predictable-CREATE-address trick as
		// runSelfDestructTruthTableCase: clause 0 of a fresh EVM, zero-value
		// TransactionContext.ID -- fund the contract's own energy before it
		// is created.
		predicted := thor.CreateContractAddress(thor.Bytes32{}, 0, 0)
		assert.Nil(t, st.SetEnergy(predicted, energy, now))
	}

	// initcode: ADDRESS; SELFDESTRUCT (toSelf) or PUSH20 <receiver>; SELFDESTRUCT.
	var initcode []byte
	if toSelf {
		initcode = []byte{0x30, 0xff}
	} else {
		initcode = append([]byte{0x73}, receiver.Bytes()...)
		initcode = append(initcode, 0xff)
	}
	clause := tx.NewClause(nil).WithValue(bal).WithData(initcode)

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &fork)
	exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)
	if !assert.NotNil(t, out.ContractAddress, "clause is a contract creation, ContractAddress must be set") {
		t.FailNow()
	}
	contractAddr := *out.ContractAddress

	snap := selfDestructDestroyPathSnapshot{
		Events:    snapshotSelfDestructEvents(out.Events),
		Transfers: snapshotSelfDestructTransfers(out.Transfers),
	}

	contractBalance, err := st.GetBalance(contractAddr)
	assert.Nil(t, err)
	contractEnergy, err := builtin.Energy.Native(st, now).Get(contractAddr)
	assert.Nil(t, err)
	contractExists, err := st.Exists(contractAddr)
	assert.Nil(t, err)
	contractCode, err := st.GetCode(contractAddr)
	assert.Nil(t, err)
	snap.ContractBalance = contractBalance.String()
	snap.ContractEnergy = contractEnergy.String()
	snap.ContractExists = contractExists
	snap.ContractCode = contractCode

	if !toSelf {
		receiverBalance, err := st.GetBalance(receiver)
		assert.Nil(t, err)
		receiverEnergy, err := builtin.Energy.Native(st, now).Get(receiver)
		assert.Nil(t, err)
		snap.ReceiverBalance = receiverBalance.String()
		snap.ReceiverEnergy = receiverEnergy.String()
		snap.ReceiverSettlementPoint = settlementPointOf(t, db, st, receiver)
	}

	return snap
}

// TestSelfDestructDestroyPathMatchesLegacy is a differential test: for any
// SELFDESTRUCT where shouldDestroy == true (contract created and destroyed
// within the same clause -- the only case that also existed before
// EIP-6780), OnSuicideContract's emitted events/transfers and the resulting
// state must be byte-for-byte identical between the legacy opcode
// (thor.NoFork, opSuicide, which hardcodes shouldDestroy=true) and the
// EIP-6780 opcode (eip6780RuntimeFork, opSuicide6780, which computes
// shouldDestroy via IsNewContract and gets true here for the same reason).
// See TestSelfDestructTransferToReceiver and TestSelfDestructTruthTable for
// independent checks of expected values.
//
// shouldDestroy == false (a pre-existing contract surviving a
// self-destruct-to-self, EIP-6780's "!toSelf" guard) has no legacy
// counterpart -- opSuicide always destroys -- so it's out of scope here; see
// TestSelfDestructTruthTable's "self, pre-existing, ... (representative,
// no-op)" case instead.
//
// The bal x energy matrix below also covers energy != 0: the original
// version of this test only varied bal, so the ERC20 Transfer log's
// legacy-vs-EIP-6780 alignment was never exercised on the destroy path.
func TestSelfDestructDestroyPathMatchesLegacy(t *testing.T) {
	nonZeroBal := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18))
	zeroBal := big.NewInt(0)
	nonZeroEnergy := big.NewInt(500)
	zeroEnergy := big.NewInt(0)

	cases := []struct {
		name   string
		toSelf bool
		bal    *big.Int
		energy *big.Int
	}{
		{name: "transfer to receiver, bal!=0 energy=0", toSelf: false, bal: nonZeroBal, energy: zeroEnergy},
		{name: "transfer to receiver, bal==0 energy=0", toSelf: false, bal: zeroBal, energy: zeroEnergy},
		{name: "self-destruct to self, bal!=0 energy=0", toSelf: true, bal: nonZeroBal, energy: zeroEnergy},
		{name: "self-destruct to self, bal==0 energy=0", toSelf: true, bal: zeroBal, energy: zeroEnergy},
		{name: "transfer to receiver, bal!=0 energy!=0", toSelf: false, bal: nonZeroBal, energy: nonZeroEnergy},
		{name: "transfer to receiver, bal==0 energy!=0", toSelf: false, bal: zeroBal, energy: nonZeroEnergy},
		{name: "self-destruct to self, bal!=0 energy!=0", toSelf: true, bal: nonZeroBal, energy: nonZeroEnergy},
		{name: "self-destruct to self, bal==0 energy!=0", toSelf: true, bal: zeroBal, energy: nonZeroEnergy},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			legacy := runSelfDestructDestroyPathCase(t, thor.NoFork, c.toSelf, c.bal, c.energy)
			eip6780 := runSelfDestructDestroyPathCase(t, eip6780RuntimeFork(), c.toSelf, c.bal, c.energy)
			assert.Equal(t, legacy, eip6780, "legacy (opSuicide) vs EIP-6780 (opSuicide6780) diverge for shouldDestroy==true case %q", c.name)
		})
	}
}

// TestSelfDestructToSelfBypassesEnergySupplyAccounting records CURRENT
// behavior (not a claim of correctness) for toSelf && shouldDestroy &&
// (bal != 0 || energy != 0): the destroyed contract's VET and VTHO both
// disappear from its own account, but Energy.TotalSupply/TotalBurned do not
// move. Those two figures are only maintained by Energy.Add/Sub writing to
// the "total-add-sub" storage item; state.State.Delete (invoked by Suicide)
// never touches it, so this destruction is invisible to supply accounting.
func TestSelfDestructToSelfBypassesEnergySupplyAccounting(t *testing.T) {
	cases := []struct {
		name   string
		bal    *big.Int
		energy *big.Int
	}{
		{name: "bal!=0 energy=0", bal: big.NewInt(1e15), energy: big.NewInt(0)},
		{name: "bal=0 energy!=0", bal: big.NewInt(0), energy: big.NewInt(500)},
		{name: "bal!=0 energy!=0", bal: big.NewInt(1e15), energy: big.NewInt(500)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := muxdb.NewMem()
			noFork := thor.NoFork
			g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
			b0, _, _, err := g.Build(state.NewStater(db))
			assert.Nil(t, err)
			repo, _ := chain.NewRepository(db, b0)
			st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

			origin := genesis.DevAccounts()[0].Address
			t0 := b0.Header().Timestamp()
			now := t0 + 3*secondsInYear

			if c.energy.Sign() != 0 {
				// see runSelfDestructTruthTableCase's createInClause branch for why
				// this address is predictable and pre-fundable.
				predicted := thor.CreateContractAddress(thor.Bytes32{}, 0, 0)
				assert.Nil(t, st.SetEnergy(predicted, c.energy, now))
			}

			beforeSupply, err := builtin.Energy.Native(st, now).TotalSupply()
			assert.Nil(t, err)
			beforeBurned, err := builtin.Energy.Native(st, now).TotalBurned()
			assert.Nil(t, err)

			// initcode: ADDRESS; SELFDESTRUCT
			initcode := []byte{0x30, 0xff}
			clause := tx.NewClause(nil).WithValue(c.bal).WithData(initcode)

			eip6780Fork := eip6780RuntimeFork()
			rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &eip6780Fork)
			exec, _ := rt.PrepareClause(clause, 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
			out, _, err := exec()
			assert.Nil(t, err)
			assert.Nil(t, out.VMErr)
			if !assert.NotNil(t, out.ContractAddress) {
				return
			}
			contractAddr := *out.ContractAddress

			exists, err := st.Exists(contractAddr)
			assert.Nil(t, err)
			balance, err := st.GetBalance(contractAddr)
			assert.Nil(t, err)
			energy, err := builtin.Energy.Native(st, now).Get(contractAddr)
			assert.Nil(t, err)
			assert.False(t, exists, "contract must be destroyed")
			assert.Zerof(t, balance.Sign(), "contract VET must be gone, actual=%s", balance)
			assert.Zerof(t, energy.Sign(), "contract VTHO must be gone, actual=%s", energy)

			afterSupply, err := builtin.Energy.Native(st, now).TotalSupply()
			assert.Nil(t, err)
			afterBurned, err := builtin.Energy.Native(st, now).TotalBurned()
			assert.Nil(t, err)

			assert.Zerof(t, new(big.Int).Sub(afterSupply, beforeSupply).Sign(),
				"TotalSupply changed across the destruction: before=%s after=%s", beforeSupply, afterSupply)
			assert.Zerof(t, new(big.Int).Sub(afterBurned, beforeBurned).Sign(),
				"TotalBurned changed across the destruction: before=%s after=%s", beforeBurned, afterBurned)
		})
	}
}

// TestSelfDestructRevertRollsBackTransfer checks that when a clause's
// SELFDESTRUCT-driven transfer is followed by a later clause that fails
// (VMErr != nil), the whole transaction is reverted -- including the
// receiver's balance, its settled energy, and its settlement point itself,
// not just the projected energy value. See runtime.go's ExecuteTransaction:
// any clause VMErr rolls the whole tx back to the checkpoint taken before
// clause 0.
func TestSelfDestructRevertRollsBackTransfer(t *testing.T) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0]
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	receiver := thor.BytesToAddress([]byte("selfdestruct-revert-receiver"))
	oldReceiverBalance := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	assert.Nil(t, st.SetBalance(receiver, oldReceiverBalance))
	assert.Nil(t, st.SetEnergy(receiver, big.NewInt(0), t0))

	drained := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18))
	// initcode: PUSH20 <receiver>; SELFDESTRUCT
	initcode := append([]byte{0x73}, receiver.Bytes()...)
	initcode = append(initcode, 0xff)

	// Clause 0: create a contract funded with `drained` VET whose initcode
	// immediately SELFDESTRUCTs, sending its balance to receiver.
	clause0 := tx.NewClause(nil).WithValue(drained).WithData(initcode)
	// Clause 1: To == nil (contract creation) with a bare INVALID opcode as
	// initcode -- guaranteed VMErr, unconditionally, regardless of fork.
	clause1 := tx.NewClause(nil).WithData([]byte{0xfe})

	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(repo.ChainTag()).
		Clause(clause0).
		Clause(clause1).
		Gas(1_000_000).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	readReceiverState := func() (balance, energyAtNow *big.Int) {
		balance, err := st.GetBalance(receiver)
		assert.Nil(t, err)
		energyAtNow, err = builtin.Energy.Native(st, now).Get(receiver)
		assert.Nil(t, err)
		return
	}

	beforeBalance, beforeEnergy := readReceiverState()

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now, GasLimit: b0.Header().GasLimit()}, &noFork)
	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	if !assert.NotNil(t, receipt) {
		return
	}
	assert.True(t, receipt.Reverted, "clause 1's INVALID opcode must revert the whole transaction")

	afterBalance, afterEnergy := readReceiverState()
	assert.Zerof(t, afterBalance.Cmp(beforeBalance), "receiver balance must be unchanged after full tx revert: before=%s after=%s", beforeBalance, afterBalance)
	assert.Zerof(t, afterEnergy.Cmp(beforeEnergy), "receiver energy must be unchanged after full tx revert: before=%s after=%s", beforeEnergy, afterEnergy)

	// Settlement point itself must roll back to t0, not stay at `now`.
	settlementPoint := settlementPointOf(t, db, st, receiver)
	assert.Equalf(t, t0, settlementPoint, "receiver settlement point must be rolled back to t0, got %d", settlementPoint)
}

// TestSelfDestructToSelfPreExistingForkBoundary demonstrates that EIP-6780
// flips the outcome of the exact same construction across the fork boundary:
// a pre-existing contract self-destructing to itself. Legacy still destroys
// it (shouldDestroy is hardcoded true) and burns the contract's VET/VTHO
// while still emitting the events that describe a transfer which never
// happened (see TestSelfDestructToSelfBypassesEnergySupplyAccounting);
// EIP-6780 hits the "toSelf && !shouldDestroy" early return and is a
// complete no-op. Same transaction, opposite result depending only on which
// side of the fork boundary it lands on -- this is EIP-6780's behavior
// break, not a bug.
func TestSelfDestructToSelfPreExistingForkBoundary(t *testing.T) {
	bal := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18)) // 500,000 VET
	contractAddr := thor.BytesToAddress([]byte("selfdestruct-preexisting-toself-fork"))

	run := func(t *testing.T, fork thor.ForkConfig) (snap selfDestructDestroyPathSnapshot, growth *big.Int) {
		t.Helper()

		db := muxdb.NewMem()
		noFork := thor.NoFork
		g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
		b0, _, _, err := g.Build(state.NewStater(db))
		assert.Nil(t, err)
		repo, _ := chain.NewRepository(db, b0)
		st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

		origin := genesis.DevAccounts()[0].Address
		t0 := b0.Header().Timestamp()
		now := t0 + 3*secondsInYear

		// ADDRESS; SELFDESTRUCT, pre-set as runtime code -- IsNewContract is
		// false regardless of fork.
		assert.Nil(t, st.SetCode(contractAddr, []byte{0x30, 0xff}))
		assert.Nil(t, st.SetBalance(contractAddr, bal))
		// Settle at t0, not now: growth over [t0, now) accrues on `bal`, giving
		// the contract a sizable accumulated VTHO balance by the time it
		// self-destructs.
		assert.Nil(t, st.SetEnergy(contractAddr, big.NewInt(0), t0))

		rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &fork)
		exec, _ := rt.PrepareClause(tx.NewClause(&contractAddr), 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
		out, _, err := exec()
		assert.Nil(t, err)
		assert.Nil(t, out.VMErr)

		snap.Events = snapshotSelfDestructEvents(out.Events)
		snap.Transfers = snapshotSelfDestructTransfers(out.Transfers)

		contractBalance, err := st.GetBalance(contractAddr)
		assert.Nil(t, err)
		contractEnergy, err := builtin.Energy.Native(st, now).Get(contractAddr)
		assert.Nil(t, err)
		contractExists, err := st.Exists(contractAddr)
		assert.Nil(t, err)
		contractCode, err := st.GetCode(contractAddr)
		assert.Nil(t, err)
		snap.ContractBalance = contractBalance.String()
		snap.ContractEnergy = contractEnergy.String()
		snap.ContractExists = contractExists
		snap.ContractCode = contractCode

		return snap, energyGrowth(bal, now-t0)
	}

	legacy, growth := run(t, thor.NoFork)
	assert.NotEqual(t, 0, growth.Sign(), "growth must be non-zero to exercise a sizable VTHO burn")
	t.Logf("accumulated VTHO burned by the legacy self-destruct-to-self: %s", growth)

	// legacy: shouldDestroy hardcoded true -> falls through past the early
	// return, reports a phantom self-transfer, then the account is deleted.
	assert.Equal(t, "0", legacy.ContractBalance, "legacy: VET must be burned")
	assert.Equal(t, "0", legacy.ContractEnergy, "legacy: VTHO must be burned")
	assert.False(t, legacy.ContractExists, "legacy: account must be deleted")
	assert.Empty(t, legacy.ContractCode, "legacy: code must be gone")

	event, _ := builtin.Energy.ABI.EventByName("Transfer")
	data, err := event.Encode(growth)
	assert.Nil(t, err)
	selfTopic := thor.BytesToBytes32(contractAddr.Bytes())
	assert.Equal(t, []selfDestructEventSnapshot{{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), selfTopic, selfTopic},
		Data:    data,
	}}, legacy.Events, "legacy: phantom ERC20 Transfer log describing the burned VTHO")
	assert.Equal(t, []selfDestructTransferSnapshot{{
		Sender:    contractAddr,
		Recipient: contractAddr,
		Amount:    bal.String(),
	}}, legacy.Transfers, "legacy: phantom tx.Transfer describing the burned VET")

	eip6780, _ := run(t, eip6780RuntimeFork())

	// EIP-6780: toSelf && !shouldDestroy hits the early return -- complete
	// no-op, nothing moved, nothing reported.
	assert.Equal(t, bal.String(), eip6780.ContractBalance, "EIP-6780: VET must survive untouched")
	assert.Equal(t, growth.String(), eip6780.ContractEnergy, "EIP-6780: VTHO must survive untouched")
	assert.True(t, eip6780.ContractExists, "EIP-6780: account must survive")
	assert.NotEmpty(t, eip6780.ContractCode, "EIP-6780: code must survive")
	assert.Empty(t, eip6780.Events, "EIP-6780: no event emitted")
	assert.Empty(t, eip6780.Transfers, "EIP-6780: no transfer recorded")
}

// TestSelfDestructPreExistingLegacyToReceiver fills in the "legacy,
// pre-existing contract, !toSelf" cell of the behavior matrix (the row is
// otherwise only exercised for EIP-6780, see runtime_test.go's "EIP-6780
// selfdestruct pre-existing contract preserves code"): under the legacy
// opcode a pre-existing contract still gets deleted -- opSuicide calls
// Suicide unconditionally, regardless of shouldDestroy -- so this is the
// only case where an asset transfer and the erasure of a contract's own code
// happen together.
func TestSelfDestructPreExistingLegacyToReceiver(t *testing.T) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	receiver := thor.BytesToAddress([]byte("selfdestruct-preexisting-legacy-receiver"))
	contractAddr := thor.BytesToAddress([]byte("selfdestruct-preexisting-legacy-contract"))

	bal := new(big.Int).Mul(big.NewInt(500_000), big.NewInt(1e18)) // 500,000 VET

	// PUSH20 <receiver>; SELFDESTRUCT, pre-set as runtime code.
	code := append([]byte{0x73}, receiver.Bytes()...)
	code = append(code, 0xff)
	assert.Nil(t, st.SetCode(contractAddr, code))
	assert.Nil(t, st.SetBalance(contractAddr, bal))
	// Settle at t0: growth over [t0, now) accrues on `bal`, so the contract
	// carries a sizable accumulated VTHO balance into the self-destruct.
	assert.Nil(t, st.SetEnergy(contractAddr, big.NewInt(0), t0))
	wantEnergy := energyGrowth(bal, now-t0)
	assert.NotEqual(t, 0, wantEnergy.Sign(), "growth must be non-zero to exercise a sizable transfer")

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &noFork)
	exec, _ := rt.PrepareClause(tx.NewClause(&contractAddr), 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	receiverBalance, err := st.GetBalance(receiver)
	assert.Nil(t, err)
	assert.Zerof(t, receiverBalance.Cmp(bal), "receiver VET: expected=%s actual=%s", bal, receiverBalance)

	receiverEnergy, err := builtin.Energy.Native(st, now).Get(receiver)
	assert.Nil(t, err)
	assert.Zerof(t, receiverEnergy.Cmp(wantEnergy), "receiver VTHO: expected=%s actual=%s", wantEnergy, receiverEnergy)

	assert.Equal(t, now, settlementPointOf(t, db, st, receiver), "receiver settlement point must advance to now")

	contractBalance, err := st.GetBalance(contractAddr)
	assert.Nil(t, err)
	contractEnergy, err := builtin.Energy.Native(st, now).Get(contractAddr)
	assert.Nil(t, err)
	contractExists, err := st.Exists(contractAddr)
	assert.Nil(t, err)
	contractCode, err := st.GetCode(contractAddr)
	assert.Nil(t, err)

	assert.Zerof(t, contractBalance.Sign(), "contract VET must be gone, actual=%s", contractBalance)
	assert.Zerof(t, contractEnergy.Sign(), "contract VTHO must be gone, actual=%s", contractEnergy)
	assert.False(t, contractExists, "legacy: pre-existing contract must still be deleted")
	assert.Empty(t, contractCode, "legacy: contract code must be erased along with the transfer")

	assert.Equal(t, 1, len(out.Events), "one ERC20 Transfer log for the moved VTHO")
	event, _ := builtin.Energy.ABI.EventByName("Transfer")
	data, err := event.Encode(wantEnergy)
	assert.Nil(t, err)
	assert.Equal(t, &tx.Event{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(contractAddr.Bytes()), thor.BytesToBytes32(receiver.Bytes())},
		Data:    data,
	}, out.Events[0])

	assert.Equal(t, 1, len(out.Transfers), "one tx.Transfer for the moved VET")
	assert.Equal(t, &tx.Transfer{Sender: contractAddr, Recipient: receiver, Amount: bal}, out.Transfers[0])
}

// TestSelfDestructRepeatedSettlementSameReceiver checks read/write
// consistency when the same receiver is settled twice within one
// transaction: the second settle() call must see the energy the first
// settle() call in the same transaction just wrote, not a stale value.
//
// A single clause can't drive two CREATE+SELFDESTRUCT sequences without
// hand-written CREATE bytecode, so this uses two clauses of one transaction
// instead: runtime.go's ExecuteTransaction shares one *state.State across
// every clause of a tx, so "twice in one clause" and "twice across clauses
// of one tx" exercise the same read-your-own-write path this test is after.
func TestSelfDestructRepeatedSettlementSameReceiver(t *testing.T) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0]
	t0 := b0.Header().Timestamp()
	now := t0 + 3*secondsInYear

	receiver := thor.BytesToAddress([]byte("selfdestruct-repeated-settlement-receiver"))
	oldBalance := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18)) // settled at t0
	assert.Nil(t, st.SetBalance(receiver, oldBalance))
	assert.Nil(t, st.SetEnergy(receiver, big.NewInt(0), t0))

	drained1 := new(big.Int).Mul(big.NewInt(300_000), big.NewInt(1e18))
	drained2 := new(big.Int).Mul(big.NewInt(200_000), big.NewInt(1e18))
	energy1 := big.NewInt(400)
	energy2 := big.NewInt(300)

	// initcode: PUSH20 <receiver>; SELFDESTRUCT
	initcode := append([]byte{0x73}, receiver.Bytes()...)
	initcode = append(initcode, 0xff)
	clause0 := tx.NewClause(nil).WithValue(drained1).WithData(initcode)
	clause1 := tx.NewClause(nil).WithValue(drained2).WithData(initcode)

	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(repo.ChainTag()).
		Clause(clause0).
		Clause(clause1).
		Gas(1_000_000).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	// Predict each clause's CREATE address (see runSelfDestructTruthTableCase's
	// comment: deterministic given txID + clauseIndex + a fresh EVM's
	// zero-valued creation counter) so each contract's own energy can be
	// pre-funded before creation.
	c1 := thor.CreateContractAddress(trx.ID(), 0, 0)
	c2 := thor.CreateContractAddress(trx.ID(), 1, 0)
	assert.Nil(t, st.SetEnergy(c1, energy1, now))
	assert.Nil(t, st.SetEnergy(c2, energy2, now))

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now, GasLimit: b0.Header().GasLimit()}, &noFork)
	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	if !assert.NotNil(t, receipt) {
		return
	}
	assert.False(t, receipt.Reverted)
	if !assert.Equal(t, 2, len(receipt.Outputs)) {
		return
	}

	// Derivation: clause 0's settle() runs first, adding growth accrued on
	// `oldBalance` over [t0, now) plus energy1, and advances the settlement
	// point to `now`. Clause 1's settle() then reads that value back --
	// growth over [now, now) is 0 -- and adds energy2 on top.
	growth := energyGrowth(oldBalance, now-t0)
	wantEnergy := new(big.Int).Add(growth, energy1)
	wantEnergy.Add(wantEnergy, energy2)
	wantBalance := new(big.Int).Add(oldBalance, drained1)
	wantBalance.Add(wantBalance, drained2)

	receiverBalance, err := st.GetBalance(receiver)
	assert.Nil(t, err)
	assert.Zerof(t, receiverBalance.Cmp(wantBalance), "receiver VET: expected=%s actual=%s", wantBalance, receiverBalance)

	receiverEnergy, err := builtin.Energy.Native(st, now).Get(receiver)
	assert.Nil(t, err)
	assert.Zerof(t, receiverEnergy.Cmp(wantEnergy), "receiver VTHO: expected=%s actual=%s", wantEnergy, receiverEnergy)

	assert.Equal(t, now, settlementPointOf(t, db, st, receiver), "receiver settlement point must be `now` after both settles")

	event, _ := builtin.Energy.ABI.EventByName("Transfer")

	checkClauseOutput := func(clauseIdx int, contractAddr thor.Address, drained, energy *big.Int) {
		out := receipt.Outputs[clauseIdx]
		if !assert.Equal(t, 2, len(out.Events), "clause %d: $Master + energy Transfer log", clauseIdx) {
			return
		}
		data, err := event.Encode(energy)
		assert.Nil(t, err)
		assert.Equal(t, &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(contractAddr.Bytes()), thor.BytesToBytes32(receiver.Bytes())},
			Data:    data,
		}, out.Events[1], "clause %d: OnSuicideContract energy Transfer log", clauseIdx)

		if !assert.Equal(t, 2, len(out.Transfers), "clause %d: creation endowment + OnSuicideContract transfer", clauseIdx) {
			return
		}
		assert.Equal(t, &tx.Transfer{Sender: contractAddr, Recipient: receiver, Amount: drained}, out.Transfers[1],
			"clause %d: OnSuicideContract VET transfer", clauseIdx)
	}
	checkClauseOutput(0, c1, drained1, energy1)
	checkClauseOutput(1, c2, drained2, energy2)
}

// TestSelfDestructChainedThroughExternalCall exercises A -> B -> C. SELFDESTRUCT
// never invokes the receiver's code, so a real chain has to be driven by an
// ordinary external CALL: the top-level clause calls B, B's own code CALLs A
// (whose code unconditionally self-destructs to B), and once that call
// returns B self-destructs to C. Legacy fork, so both A and B end up deleted
// once their balance/energy have hopped downstream.
func TestSelfDestructChainedThroughExternalCall(t *testing.T) {
	db := muxdb.NewMem()
	noFork := thor.NoFork
	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &noFork})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	origin := genesis.DevAccounts()[0].Address
	now := b0.Header().Timestamp() + 3*secondsInYear

	a := thor.BytesToAddress([]byte("selfdestruct-chain-a"))
	b := thor.BytesToAddress([]byte("selfdestruct-chain-b"))
	c := thor.BytesToAddress([]byte("selfdestruct-chain-c"))

	balA := new(big.Int).Mul(big.NewInt(100_000), big.NewInt(1e18))
	balB := new(big.Int).Mul(big.NewInt(50_000), big.NewInt(1e18))
	energyA := big.NewInt(400)
	energyB := big.NewInt(300)

	// A: PUSH20 <b>; SELFDESTRUCT -- ignores calldata, always destructs to B.
	codeA := append([]byte{0x73}, b.Bytes()...)
	codeA = append(codeA, 0xff)
	assert.Nil(t, st.SetCode(a, codeA))
	assert.Nil(t, st.SetBalance(a, balA))
	// Settled at `now`, not t0: pins each contract's own energy to an exact
	// value, keeping this test about propagation rather than growth (see
	// TestSelfDestructToSelfPreExistingForkBoundary and
	// TestSelfDestructPreExistingLegacyToReceiver for the growth cases).
	assert.Nil(t, st.SetEnergy(a, energyA, now))

	// B: zero-value CALL(gas, a, 0, 0, 0, 0, 0); POP; PUSH20 <c>; SELFDESTRUCT.
	// Stack push order for CALL matches opCall's pop order (gas, addr, value,
	// argsOffset, argsSize, retOffset, retSize), reversed.
	codeB := []byte{
		0x60, 0x00, // PUSH1 0 (retSize)
		0x60, 0x00, // PUSH1 0 (retOffset)
		0x60, 0x00, // PUSH1 0 (argsSize)
		0x60, 0x00, // PUSH1 0 (argsOffset)
		0x60, 0x00, // PUSH1 0 (value)
	}
	codeB = append(codeB, 0x73)
	codeB = append(codeB, a.Bytes()...)     // PUSH20 <a> (address)
	codeB = append(codeB, 0x5a, 0xf1, 0x50) // GAS; CALL; POP (discard success flag)
	codeB = append(codeB, 0x73)
	codeB = append(codeB, c.Bytes()...) // PUSH20 <c>
	codeB = append(codeB, 0xff)         // SELFDESTRUCT
	assert.Nil(t, st.SetCode(b, codeB))
	assert.Nil(t, st.SetBalance(b, balB))
	assert.Nil(t, st.SetEnergy(b, energyB, now))

	rt := runtime.New(repo.NewChain(b0.Header().ID()), st, &xenv.BlockContext{Time: now}, &noFork)
	exec, _ := rt.PrepareClause(tx.NewClause(&b), 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	wantBal := new(big.Int).Add(balA, balB)
	wantEnergy := new(big.Int).Add(energyA, energyB)

	cBalance, err := st.GetBalance(c)
	assert.Nil(t, err)
	assert.Zerof(t, cBalance.Cmp(wantBal), "C VET: expected=%s actual=%s", wantBal, cBalance)

	cEnergy, err := builtin.Energy.Native(st, now).Get(c)
	assert.Nil(t, err)
	assert.Zerof(t, cEnergy.Cmp(wantEnergy), "C VTHO: expected=%s actual=%s", wantEnergy, cEnergy)

	assert.Equal(t, now, settlementPointOf(t, db, st, c), "C settlement point must be `now`")

	for _, addr := range []struct {
		name string
		addr thor.Address
	}{{"A", a}, {"B", b}} {
		balance, err := st.GetBalance(addr.addr)
		assert.Nil(t, err)
		energy, err := builtin.Energy.Native(st, now).Get(addr.addr)
		assert.Nil(t, err)
		exists, err := st.Exists(addr.addr)
		assert.Nil(t, err)
		code, err := st.GetCode(addr.addr)
		assert.Nil(t, err)
		assert.Zerof(t, balance.Sign(), "%s VET must be gone, actual=%s", addr.name, balance)
		assert.Zerof(t, energy.Sign(), "%s VTHO must be gone, actual=%s", addr.name, energy)
		assert.False(t, exists, "%s must be deleted (legacy fork)", addr.name)
		assert.Empty(t, code, "%s code must be erased", addr.name)
	}

	// A -> B (inside the CALL) then B -> C (outer frame) -- the only two
	// OnSuicideContract side effects: no $Master events (no CREATE happened)
	// and no CALL-value transfer (value was 0).
	if !assert.Equal(t, 2, len(out.Events)) {
		return
	}
	if !assert.Equal(t, 2, len(out.Transfers)) {
		return
	}

	event, _ := builtin.Energy.ABI.EventByName("Transfer")
	dataAB, err := event.Encode(energyA)
	assert.Nil(t, err)
	assert.Equal(t, &tx.Event{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(a.Bytes()), thor.BytesToBytes32(b.Bytes())},
		Data:    dataAB,
	}, out.Events[0], "A -> B energy transfer log")

	dataBC, err := event.Encode(wantEnergy)
	assert.Nil(t, err)
	assert.Equal(t, &tx.Event{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(b.Bytes()), thor.BytesToBytes32(c.Bytes())},
		Data:    dataBC,
	}, out.Events[1], "B -> C energy transfer log")

	assert.Equal(t, &tx.Transfer{Sender: a, Recipient: b, Amount: balA}, out.Transfers[0], "A -> B VET transfer")
	assert.Equal(t, &tx.Transfer{Sender: b, Recipient: c, Amount: wantBal}, out.Transfers[1], "B -> C VET transfer")
}
