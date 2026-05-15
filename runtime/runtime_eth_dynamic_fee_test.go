// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gomath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

// setupEthTxRuntime builds a devnet with GALACTICA active at block 1 and
// returns (repo at b1, fresh state at b0, baseFee at b1, b1 timestamp).
// The state isn't advanced past b0 because b1 has no txs; ctx.BaseFee /
// ctx.Number / ctx.Time supplied at runtime time are sufficient to land
// in the post-galactica branch of runtime.go.
func setupEthTxRuntime(t *testing.T) (*chain.Repository, *state.State, *big.Int, uint64) {
	t.Helper()
	db := muxdb.NewMem()

	fc := &thor.SoloFork
	hayabusaTP := uint32(math.MaxUint32)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	fc.HAYABUSA = math.MaxUint32
	fc.GALACTICA = 1

	g := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: fc})
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)
	repo, _ := chain.NewRepository(db, b0)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	ver := trie.Version{Major: b0.Header().Number() + 1, Minor: 0}
	stg, err := st.Stage(ver)
	assert.Nil(t, err)
	root, err := stg.Commit()
	assert.Nil(t, err)

	baseFee := big.NewInt(thor.InitialBaseFee)
	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		Timestamp(b0.Header().Timestamp() + thor.BlockInterval()).
		GasLimit(b0.Header().GasLimit()).
		BaseFee(baseFee).
		StateRoot(root).
		Build()
	repo.AddBlock(b1, nil, 0, true)

	st = state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	return repo, st, baseFee, b1.Header().Timestamp()
}

func TestEthDynFee_PlainTransfer(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	value := big.NewInt(1000)
	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)
	gas := uint64(21000)

	addr := recipient.Address
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(gas).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(1).
		To(&addr).Value(value).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	prevOriginEnergy, err := builtin.Energy.Native(st, blockTime).Get(origin.Address)
	assert.Nil(t, err)
	prevBeneficiaryEnergy, err := builtin.Energy.Native(st, blockTime).Get(beneficiary)
	assert.Nil(t, err)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	assert.False(t, receipt.Reverted, "plain transfer must not revert")

	// Intrinsic gas: pure transfer with empty data → 21000.
	assert.Equal(t, uint64(21000), receipt.GasUsed)

	// Tip routing: priorityFee × gasUsed → beneficiary.
	currBeneficiaryEnergy, err := builtin.Energy.Native(st, blockTime).Get(beneficiary)
	assert.Nil(t, err)
	tipDelta := new(big.Int).Sub(currBeneficiaryEnergy, prevBeneficiaryEnergy)
	expectedTip := new(big.Int).Mul(maxPriority, big.NewInt(int64(receipt.GasUsed)))
	assert.Equal(t, expectedTip, tipDelta, "tip = maxPriority × gasUsed")

	// Origin paid: gasUsed × effectiveGasPrice.
	currOriginEnergy, err := builtin.Energy.Native(st, blockTime).Get(origin.Address)
	assert.Nil(t, err)
	originDelta := new(big.Int).Sub(prevOriginEnergy, currOriginEnergy)
	expectedPaid := new(big.Int).Mul(maxFee, big.NewInt(int64(receipt.GasUsed)))
	assert.Equal(t, expectedPaid, originDelta, "origin paid = effectiveGasPrice × gasUsed")
	assert.Equal(t, expectedPaid, receipt.Paid)
	assert.Equal(t, origin.Address, receipt.GasPayer)
}

func TestEthDynFee_ContractCreation(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// Minimal valid creation bytecode: STOP. Empty deployed code, dataGas = 4 (one zero byte).
	code := []byte{0x00}

	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(100000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(2).
		Data(code). // nil To → contract creation
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	assert.False(t, receipt.Reverted, "creation must not revert")

	// Intrinsic gas floor: TxGas + ClauseGasContractCreation + dataGas(0x00)
	// = 5000 + 48000 + 4 = 53004. gasUsed >= floor.
	assert.GreaterOrEqual(t, receipt.GasUsed, uint64(53004))

	// Eth tx uses Ethereum's nonce-based rule: CreateAddress(origin, nonce-before-increment).
	// On-state nonce starts at 0 for the genesis account.
	assert.Len(t, receipt.Outputs, 1)
	expectedAddr := thor.Address(crypto.CreateAddress(common.Address(origin.Address), 0))
	exists, existsErr := st.Exists(expectedAddr)
	assert.Nil(t, existsErr)
	assert.True(t, exists, "contract account must exist at eth-derived address")
}

// TestEthDynFee_RevertPreservesNonce guards eth tx revert semantics: when a
// clause hits VMErr, the receipt is marked reverted but the sender nonce
// increment must persist (matches Ethereum: failed txs still consume nonce).
func TestEthDynFee_RevertPreservesNonce(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// Init code = INVALID opcode → ErrInvalidOpCode → VMErr.
	// CREATE path: evm.create increments nonce before its snapshot, so the
	// increment is preserved across EVM-internal RevertToSnapshot.
	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(100000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(0).
		Data([]byte{0xfe}).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	prevNonce, err := st.GetNonce(origin.Address)
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), prevNonce)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err, "ExecuteTransaction should not error on clause-level revert")
	assert.True(t, receipt.Reverted, "eth tx must be marked reverted on VMErr")
	assert.Nil(t, receipt.Outputs, "outputs must be cleared on revert")

	// Nonce persists post-revert (Ethereum semantics).
	currNonce, err := st.GetNonce(origin.Address)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), currNonce, "sender nonce must increment even on reverted eth tx")

	// Contract account must NOT exist at the eth-derived address.
	contractAddr := thor.Address(crypto.CreateAddress(common.Address(origin.Address), 0))
	exists, err := st.Exists(contractAddr)
	assert.Nil(t, err)
	assert.False(t, exists, "failed init must not leave a contract account")
}

func TestEthDynFee_SponsoredCall(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	sponsor := genesis.DevAccounts()[2]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// Set up Prototype contract as a sponsored target where origin is a user
	// and sponsor is the selected sponsor.
	target := builtin.Prototype.Address
	bind := builtin.Prototype.Native(st).Bind(target)
	err := bind.SetCreditPlan(gomath.MaxBig256, big.NewInt(1000))
	assert.Nil(t, err)
	err = bind.AddUser(origin.Address, blockTime)
	assert.Nil(t, err)
	err = bind.Sponsor(sponsor.Address, true)
	assert.Nil(t, err)
	bind.SelectSponsor(sponsor.Address)

	// Fund sponsor with enough energy to cover gas.
	builtin.Energy.Native(st, blockTime).Add(sponsor.Address, gomath.MaxBig256)

	prevSponsorEnergy, err := builtin.Energy.Native(st, blockTime).Get(sponsor.Address)
	assert.Nil(t, err)
	prevOriginEnergy, err := builtin.Energy.Native(st, blockTime).Get(origin.Address)
	assert.Nil(t, err)

	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	// Encode a no-op-ish call: Prototype.master(target). Read-only, gas-cheap.
	method, found := builtin.Prototype.ABI.MethodByName("master")
	assert.True(t, found)
	callData, err := method.EncodeInput(target)
	assert.Nil(t, err)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(100000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(3).
		To(&target).Data(callData).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	assert.False(t, receipt.Reverted)

	// Sponsor pays.
	assert.Equal(t, sponsor.Address, receipt.GasPayer)
	currSponsorEnergy, err := builtin.Energy.Native(st, blockTime).Get(sponsor.Address)
	assert.Nil(t, err)
	assert.True(t, currSponsorEnergy.Cmp(prevSponsorEnergy) < 0, "sponsor energy decreased")

	// Origin DOES NOT pay.
	currOriginEnergy, err := builtin.Energy.Native(st, blockTime).Get(origin.Address)
	assert.Nil(t, err)
	assert.Equal(t, prevOriginEnergy, currOriginEnergy, "origin energy unchanged")
}

func TestEthDynFee_BaseFeeFloor(t *testing.T) {
	_, st, baseFee, _ := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	addr := genesis.DevAccounts()[1].Address

	// maxFee BELOW baseFee.
	maxFee := new(big.Int).Sub(baseFee, big.NewInt(1))

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(21000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(big.NewInt(0)).
		ChainID(0).
		Nonce(4).
		To(&addr).Value(big.NewInt(1)).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	resolved, err := runtime.ResolveTransaction(trx)
	assert.Nil(t, err, "resolution itself should pass — baseFee is checked at BuyGas, not resolution")

	_, _, _, _, _, err = resolved.BuyGas(st, 0, baseFee)
	assert.ErrorContains(t, err, "gas price is less than block base fee")
}

func TestEthDynFee_InsufficientBalance(t *testing.T) {
	_, st, baseFee, blockTime := setupEthTxRuntime(t)

	// Fresh key with zero energy — devnet DevAccounts have huge balances, so we need a new account.
	pk, err := crypto.GenerateKey()
	assert.Nil(t, err)
	pauper := thor.Address(crypto.PubkeyToAddress(pk.PublicKey))

	// Sanity: pauper has zero energy.
	pauperEnergy, err := builtin.Energy.Native(st, blockTime).Get(pauper)
	assert.Nil(t, err)
	assert.Equal(t, 0, pauperEnergy.Sign())

	addr := genesis.DevAccounts()[1].Address
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(21000).
		MaxFeePerGas(new(big.Int).Mul(baseFee, big.NewInt(2))).
		MaxPriorityFeePerGas(new(big.Int).Set(baseFee)).
		ChainID(0).
		Nonce(5).
		To(&addr).Value(big.NewInt(1)).
		Build()
	trx = tx.MustSign(trx, pk)

	resolved, err := runtime.ResolveTransaction(trx)
	assert.Nil(t, err)

	_, _, _, _, _, err = resolved.BuyGas(st, blockTime, baseFee)
	assert.ErrorContains(t, err, "insufficient energy")
}

// TestEthDynFee_GasCapEIP7825 verifies that the EIP-7825 per-tx gas cap
// (runtime/runtime.go: trx.Gas() > thor.MaxTxGasLimit) is enforced for eth
// tx the same as for VeChain-native tx.
func TestEthDynFee_GasCapEIP7825(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))
	recipient := genesis.DevAccounts()[1].Address

	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(thor.MaxTxGasLimit + 1).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(0).
		To(&recipient).Value(big.NewInt(1)).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    thor.MaxTxGasLimit + 100,
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	_, err := rt.ExecuteTransaction(trx)
	assert.ErrorContains(t, err, "tx gas limit exceeds the maximum allowed")
}

// TestEthDynFee_TransientStorageIsolation verifies that EIP-1153 transient
// storage is cleared between separate eth tx executions. Two eth txs each
// call a contract that TSTORE+TLOAD the same key; both must return the
// stored value (1153) — neither sees stale state from the other. A third
// eth tx that only TLOAD without prior TSTORE must read 0, proving the
// stackedmap-backed transient storage is a per-call/per-tx fresh slate.
func TestEthDynFee_TransientStorageIsolation(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// tstore(1, 1153); tload(1); mstore(0x80,_); return(0x80, 0x20)
	codeStoreLoad := []byte{
		byte(vm.PUSH2), 0x04, 0x81,
		byte(vm.PUSH1), 0x1, byte(vm.TSTORE),
		byte(vm.PUSH1), 0x1, byte(vm.TLOAD),
		byte(vm.PUSH1), 0x80, byte(vm.MSTORE),
		byte(vm.PUSH1), 0x20, byte(vm.PUSH1), 0x80, byte(vm.RETURN),
	}
	// tload(1); mstore(0x80,_); return(0x80, 0x20)
	codeLoadOnly := []byte{
		byte(vm.PUSH1), 0x1, byte(vm.TLOAD),
		byte(vm.PUSH1), 0x80, byte(vm.MSTORE),
		byte(vm.PUSH1), 0x20, byte(vm.PUSH1), 0x80, byte(vm.RETURN),
	}

	target := thor.BytesToAddress([]byte("transient-target"))

	// PrepareClause is called per (clause × tx). Each call constructs a fresh
	// stackedmap-backed statedb, so transient storage is naturally isolated.
	// We exercise the eth-tx code path explicitly via Type=TypeEthDynamicFee.
	mkExec := func(code []byte, txID thor.Bytes32) (*runtime.Output, error) {
		require := assert.New(t)
		require.NoError(st.SetCode(target, code))
		exec, _ := runtime.New(
			repo.NewChain(repo.BestBlockSummary().Header.ID()),
			st,
			&xenv.BlockContext{
				Time:        blockTime,
				Number:      repo.BestBlockSummary().Header.Number() + 1,
				GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
				BaseFee:     baseFee,
				Beneficiary: beneficiary,
			},
			&thor.SoloFork,
		).PrepareClause(
			tx.NewClause(&target),
			0,
			gomath.MaxBig256.Uint64(),
			&xenv.TransactionContext{ID: txID, Origin: origin.Address, Type: tx.TypeEthDynamicFee},
		)
		out, _, err := exec()
		return out, err
	}

	// tx 1 — TSTORE then TLOAD same key: returns 1153 from the in-flight slot.
	out1, err := mkExec(codeStoreLoad, thor.BytesToBytes32([]byte("eth-tx-1")))
	assert.NoError(t, err)
	assert.Nil(t, out1.VMErr)
	assert.Equal(t, uint64(1153), new(big.Int).SetBytes(out1.Data).Uint64(),
		"first eth tx must observe its own TSTORE")

	// tx 2 — TLOAD only (no TSTORE in this tx): must return 0, proving the
	// previous tx's transient slot was discarded at tx boundary.
	out2, err := mkExec(codeLoadOnly, thor.BytesToBytes32([]byte("eth-tx-2")))
	assert.NoError(t, err)
	assert.Nil(t, out2.VMErr)
	assert.Equal(t, uint64(0), new(big.Int).SetBytes(out2.Data).Uint64(),
		"second eth tx must NOT see prior tx's transient storage")
}

// TestEthDynFee_SelfdestructPreExisting verifies EIP-6780 dispatch on the
// eth tx path: a pre-existing contract (deployed before this tx, so
// IsNewContract→false at SUICIDE) must have its code preserved per EIP-6780,
// only its balance is transferred. This proves the eth tx code path goes
// through the same opSuicide6780 logic as VeChain-native txs.
//
// Constructor of the test contract is irrelevant — we install runtime
// bytecode directly via state.SetCode. Runtime: CALLER SELFDESTRUCT.
func TestEthDynFee_SelfdestructPreExisting(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// Pre-deploy a contract whose runtime code is `CALLER SELFDESTRUCT`.
	// IsNewContract=false at the SUICIDE site → EIP-6780 shouldDestruct=false.
	target := thor.BytesToAddress([]byte("preexisting-killable"))
	runtimeCode := []byte{0x33, 0xff} // CALLER, SELFDESTRUCT
	assert.NoError(t, st.SetCode(target, runtimeCode))
	assert.NoError(t, st.SetBalance(target, big.NewInt(2000)))

	codeBefore, err := st.GetCode(target)
	assert.NoError(t, err)
	assert.NotEmpty(t, codeBefore)

	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(100000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(0).
		To(&target). // call into pre-existing contract
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	assert.False(t, receipt.Reverted, "selfdestruct must not revert")

	// EIP-6780: pre-existing → shouldDestruct=false → Suicide() NOT called →
	// code persists.
	codeAfter, err := st.GetCode(target)
	assert.NoError(t, err)
	assert.Equal(t, codeBefore, codeAfter,
		"EIP-6780 (eth tx path): pre-existing contract code must persist after SELFDESTRUCT")

	// Balance is moved to caller.
	bal, err := st.GetBalance(target)
	assert.NoError(t, err)
	assert.Zero(t, bal.Sign(), "balance must be 0 after SELFDESTRUCT")
}

// TestEthDynFee_SelfdestructSameTxEIP6780 verifies EIP-6780 same-tx detection
// on the eth tx path. Init code is `CALLER SELFDESTRUCT` (no RETURN); during
// init execution IsNewContract→true, so shouldDestruct=true and the
// pre-funded constructor value is forwarded to the caller. A successful
// selfdestruct emits a Transfer record from the deployed addr to origin.
func TestEthDynFee_SelfdestructSameTxEIP6780(t *testing.T) {
	repo, st, baseFee, blockTime := setupEthTxRuntime(t)

	origin := genesis.DevAccounts()[0]
	beneficiary := thor.BytesToAddress([]byte("proposer"))

	// Init code: CALLER SELFDESTRUCT. No RETURN → no runtime code stored;
	// the SELFDESTRUCT fires while IsNewContract=true.
	initcode := []byte{0x33, 0xff}

	maxFee := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxPriority := new(big.Int).Set(baseFee)

	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		Gas(200000).
		MaxFeePerGas(maxFee).
		MaxPriorityFeePerGas(maxPriority).
		ChainID(0).
		Nonce(0).
		Value(big.NewInt(1234)).Data(initcode).
		Build()
	trx = tx.MustSign(trx, origin.PrivateKey)

	rt := runtime.New(
		repo.NewChain(repo.BestBlockSummary().Header.ID()),
		st,
		&xenv.BlockContext{
			Time:        blockTime,
			Number:      repo.BestBlockSummary().Header.Number() + 1,
			GasLimit:    repo.BestBlockSummary().Header.GasLimit(),
			BaseFee:     baseFee,
			Beneficiary: beneficiary,
		},
		&thor.SoloFork,
	)

	receipt, err := rt.ExecuteTransaction(trx)
	assert.Nil(t, err)
	assert.False(t, receipt.Reverted, "init+selfdestruct must not revert")

	// Eth tx CREATE address: keccak256(rlp([origin, nonce=0])).
	deployedAddr := thor.Address(crypto.CreateAddress(common.Address(origin.Address), 0))

	// EIP-6780 same-tx → Suicide() called.
	// The deployed addr's balance was funded by msg.value at CREATE time, then
	// transferred to origin by SELFDESTRUCT. Origin's balance net-delta is 0
	// (out then back), so observe via the deployed addr's terminal balance and
	// the Transfer record emitted by the suicide.
	deployedBal, err := st.GetBalance(deployedAddr)
	assert.NoError(t, err)
	assert.Zero(t, deployedBal.Sign(), "deployed contract must have 0 balance after self-destruct")

	// Two transfer records on the single clause: (origin → deployed) at CREATE
	// pre-funding, then (deployed → origin) at SELFDESTRUCT. The second is the
	// signature of EIP-6780 same-tx → Suicide() executing.
	require.Len(t, receipt.Outputs, 1)
	require.Len(t, receipt.Outputs[0].Transfers, 2)
	suicide := receipt.Outputs[0].Transfers[1]
	assert.Equal(t, deployedAddr, suicide.Sender)
	assert.Equal(t, origin.Address, suicide.Recipient)
	assert.Equal(t, big.NewInt(1234), suicide.Amount)
}
