// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

var (
	errReverted = vm.ErrExecutionReverted
	revertABI   = []byte(`[{"name": "Error","type": "function","inputs": [{"name": "message","type": "string"}]}]`)
)

type ctest struct {
	rt         *runtime.Runtime
	abi        *abi.ABI
	to, caller thor.Address
}

type ccase struct {
	rt         *runtime.Runtime
	abi        *abi.ABI
	to, caller thor.Address
	name       string
	args       []any
	events     tx.Events
	provedWork *big.Int
	txID       thor.Bytes32
	blockRef   tx.BlockRef
	gasPayer   thor.Address
	expiration uint32
	value      *big.Int

	output          *[]any
	vmerr           error
	gas             uint64
	revertErrorName string
}

func (c *ctest) Case(name string, args ...any) *ccase {
	return &ccase{
		rt:     c.rt,
		abi:    c.abi,
		to:     c.to,
		caller: c.caller,
		name:   name,
		args:   args,
	}
}

func (c *ccase) To(to thor.Address) *ccase {
	c.to = to
	return c
}

func (c *ccase) Caller(caller thor.Address) *ccase {
	c.caller = caller
	return c
}

func (c *ccase) Value(value *big.Int) *ccase {
	c.value = value
	return c
}

func (c *ccase) ProvedWork(provedWork *big.Int) *ccase {
	c.provedWork = provedWork
	return c
}

func (c *ccase) TxID(txID thor.Bytes32) *ccase {
	c.txID = txID
	return c
}

func (c *ccase) BlockRef(blockRef tx.BlockRef) *ccase {
	c.blockRef = blockRef
	return c
}

func (c *ccase) GasPayer(gasPayer thor.Address) *ccase {
	c.gasPayer = gasPayer
	return c
}

func (c *ccase) Expiration(expiration uint32) *ccase {
	c.expiration = expiration
	return c
}

func (c *ccase) ShouldVMError(err error) *ccase {
	c.vmerr = err
	return c
}

func (c *ccase) ShouldLog(events ...*tx.Event) *ccase {
	c.events = events
	return c
}

func (c *ccase) ShouldOutput(outputs ...any) *ccase {
	c.output = &outputs
	return c
}

func (c *ccase) ShouldUseGas(gas uint64) *ccase {
	c.gas = gas
	return c
}

func (c *ccase) ShouldRevertError(name string) *ccase {
	c.revertErrorName = name
	c.vmerr = errReverted
	return c
}

func (c *ccase) Assert(t *testing.T) *ccase {
	method, ok := c.abi.MethodByName(c.name)
	assert.True(t, ok, "should have method")

	constant := method.Const()
	stage, err := c.rt.State().Stage(trie.Version{})
	assert.Nil(t, err, "should stage state")
	stateRoot := stage.Hash()

	data, err := method.EncodeInput(c.args...)
	assert.Nil(t, err, "should encode input")

	clause := tx.NewClause(&c.to).WithData(data)
	if c.value != nil {
		clause = clause.WithValue(c.value)
	}

	inputGas := uint64(40_000_000)
	exec, _ := c.rt.PrepareClause(clause,
		0, inputGas, &xenv.TransactionContext{
			ID:         c.txID,
			Origin:     c.caller,
			GasPrice:   &big.Int{},
			GasPayer:   c.gasPayer,
			ProvedWork: c.provedWork,
			BlockRef:   c.blockRef,
			Expiration: c.expiration,
		})
	vmout, _, err := exec()
	assert.Nil(t, err)
	if constant || vmout.VMErr != nil {
		stage, err := c.rt.State().Stage(trie.Version{})
		assert.Nil(t, err, "should stage state")
		newStateRoot := stage.Hash()
		assert.Equal(t, stateRoot, newStateRoot)
	}
	if c.vmerr != nil {
		assert.Equal(t, c.vmerr, vmout.VMErr)
	} else {
		if vmout.VMErr != nil {
			t.Logf("VM output: 0x%x", vmout.Data)
		}
		assert.Nil(t, vmout.VMErr)
	}

	if c.output != nil {
		out, err := method.EncodeOutput((*c.output)...)
		assert.Nil(t, err, "should encode output")
		assert.Equal(t, out, vmout.Data, "should match output")
	}

	if len(c.events) > 0 {
		for _, ev := range c.events {
			found := func() bool {
				for _, outEv := range vmout.Events {
					if reflect.DeepEqual(ev, outEv) {
						return true
					}
				}
				return false
			}()
			assert.True(t, found, "event should appear")
		}
	}

	if c.revertErrorName != "" {
		abis, err := abi.New(fmt.Appendf(nil, `[{"name":"%s","type":"function","inputs":[]}]`, c.revertErrorName))
		assert.NoError(t, err)
		method, ok := abis.MethodByName(c.revertErrorName)
		assert.True(t, ok)
		// revert payload: selector(4) + args
		methodID := method.ID()
		methodIDBytes := methodID[:]
		assert.True(t, bytes.Equal(methodIDBytes, vmout.Data[:4]), "unexpected custom error selector")
	}

	if c.gas != 0 {
		assert.Greater(t, inputGas, vmout.LeftOverGas)
		assert.Equal(t, c.gas, inputGas-vmout.LeftOverGas, "expected = %d, got = %d", c.gas, inputGas-vmout.LeftOverGas)
	}

	c.output = nil
	c.vmerr = nil
	c.events = nil
	c.gas = 0

	return c
}

func buildGenesis(db *muxdb.MuxDB, proc func(state *state.State) error) *block.Block {
	blk, _, _, err := new(genesis.Builder).
		Timestamp(uint64(time.Now().Unix())).
		State(proc).
		ForkConfig(&thor.NoFork).
		Build(state.NewStater(db))
	if err != nil {
		panic(err)
	}
	return blk
}

func TestStakerContract_Native_CheckStake(t *testing.T) {
	var (
		caller     = genesis.DevAccounts()[0].Address
		master     = thor.BytesToAddress([]byte("master"))
		validation = thor.BytesToBytes32([]byte("validation"))
		delegator  = thor.Address{}
	)

	fc := &thor.SoloFork
	fc.HAYABUSA = 1
	hayabusaTP := uint32(2)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	var err error
	thorChain, err := testchain.NewWithFork(fc, 180)
	assert.NoError(t, err)
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))
	assert.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	thorChain.MintClauses(genesis.DevAccounts()[0], []*tx.Clause{
		tx.NewClause(&delegator).WithValue(big.NewInt(1e18)),
	})

	rt := runtime.New(
		thorChain.Repo().NewBestChain(),
		thorChain.Stater().NewState(thorChain.Repo().BestBlockSummary().Root()),
		&xenv.BlockContext{Time: thorChain.Repo().BestBlockSummary().Header.Timestamp()},
		thorChain.GetForkConfig(),
	)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Staker.ABI,
		to:     builtin.Staker.Address,
		caller: builtin.Staker.Address,
	}

	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(big.NewInt(0)).
		Caller(caller).
		ShouldRevertError("StakeIsEmpty").
		Assert(t)

	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(big.NewInt(1)).
		Caller(caller).
		ShouldRevertError("StakeIsNotMultipleOf1VET").
		Assert(t)

	test.Case("increaseStake", validation).
		Value(big.NewInt(0)).
		Caller(caller).
		ShouldRevertError("StakeIsEmpty").
		Assert(t)

	test.Case("increaseStake", validation).
		Value(big.NewInt(1)).
		Caller(caller).
		ShouldRevertError("StakeIsNotMultipleOf1VET").
		Assert(t)

	test.Case("decreaseStake", validation, big.NewInt(0)).
		Caller(caller).
		ShouldRevertError("StakeIsEmpty").
		Assert(t)

	test.Case("decreaseStake", validation, big.NewInt(1)).
		Caller(caller).
		ShouldRevertError("StakeIsNotMultipleOf1VET").
		Assert(t)

	test.Case("addDelegation", validation, uint8(100)).
		Caller(delegator).
		Value(big.NewInt(0)).
		ShouldRevertError("StakeIsEmpty").
		Assert(t)

	test.Case("addDelegation", validation, uint8(100)).
		Caller(delegator).
		Value(big.NewInt(1)).
		ShouldRevertError("StakeIsNotMultipleOf1VET").
		Assert(t)
}

func toWei(vet uint64) *big.Int {
	return big.NewInt(0).Mul(big.NewInt(int64(vet)), big.NewInt(1e18))
}

func increaseStakerBal(state *state.State, amount uint64) {
	amt := toWei(amount)

	// update effectiveVET tracking
	effectiveVETBytes, err := state.GetStorage(builtin.Staker.Address, thor.Bytes32{})
	if err != nil {
		panic(err)
	}
	effectiveVET := new(big.Int).SetBytes(effectiveVETBytes.Bytes())
	effectiveVET.Add(effectiveVET, amt)
	state.SetStorage(builtin.Staker.Address, thor.Bytes32{}, thor.BytesToBytes32(effectiveVET.Bytes()))

	// update actual contract balance
	balance, err := state.GetBalance(builtin.Staker.Address)
	if err != nil {
		panic(err)
	}
	newBalance := new(big.Int).Add(balance, amt)
	if err = state.SetBalance(builtin.Staker.Address, newBalance); err != nil {
		panic(err)
	}
}

func decreaseStakerBal(state *state.State, amount uint64) {
	amt := toWei(amount)

	// update effectiveVET tracking
	effectiveVETBytes, err := state.GetStorage(builtin.Staker.Address, thor.Bytes32{})
	if err != nil {
		panic(err)
	}
	effectiveVET := new(big.Int).SetBytes(effectiveVETBytes.Bytes())
	if effectiveVET.Cmp(amt) < 0 {
		panic(fmt.Errorf("insufficient effectiveVET: have %s, need %d", effectiveVET, amount))
	}
	effectiveVET.Sub(effectiveVET, amt)
	state.SetStorage(builtin.Staker.Address, thor.Bytes32{}, thor.BytesToBytes32(effectiveVET.Bytes()))

	// update actual contract balance
	balance, err := state.GetBalance(builtin.Staker.Address)
	if err != nil {
		panic(err)
	}
	if balance.Cmp(amt) < 0 {
		panic(fmt.Errorf("insufficient balance: have %s, need %d", balance, amount))
	}
	newBalance := new(big.Int).Sub(balance, amt)
	if err = state.SetBalance(builtin.Staker.Address, newBalance); err != nil {
		panic(err)
	}
}

func TestStakerContract_PauseSwitches(t *testing.T) {
	var (
		endorser   = thor.BytesToAddress([]byte("endorser"))
		rich       = thor.BytesToAddress([]byte("rich"))
		master     = thor.BytesToAddress([]byte("master"))
		delegator  = thor.BytesToAddress([]byte("delegator"))
		validator1 = thor.BytesToAddress([]byte("validator1"))
		validator3 = thor.BytesToAddress([]byte("validator3")) // exit

		minStakeVET = staker.MinStakeVET
		minStake    = staker.MinStake
	)

	fc := &thor.SoloFork
	fc.HAYABUSA = 0
	hayabusaTP := uint32(0)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	db := muxdb.NewMem()

	gene := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Staker.Address, builtin.Staker.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())

		builtin.Params.Native(state).Set(thor.KeyMaxBlockProposers, big.NewInt(1))
		builtin.Params.Native(state).Set(thor.KeyDelegatorContractAddress, new(big.Int).SetBytes(delegator.Bytes()))
		// pause both staker and delegator
		builtin.Params.Native(state).Set(thor.KeyStakerSwitches, big.NewInt(0b11))

		stakerNative := builtin.Staker.Native(state)
		valStake := minStakeVET * 2

		increaseStakerBal(state, valStake)
		err := stakerNative.AddValidation(validator1, endorser, thor.LowStakingPeriod(), valStake)
		if err != nil {
			return err
		}

		// add delegation1 to validator1
		increaseStakerBal(state, minStakeVET)
		_, err = stakerNative.AddDelegation(validator1, minStakeVET, 100, 10)
		if err != nil {
			return err
		}

		state.SetBalance(endorser, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))
		state.SetBalance(rich, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))
		state.SetBalance(delegator, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))

		status, err := stakerNative.SyncPOS(fc, 0)
		if err != nil {
			return err
		}
		if !status.Active {
			return errors.New("transition failed")
		}
		if stakerNative.ContractBalanceCheck(0) != nil {
			return errors.New("staker sanity check failed")
		}

		return nil
	})

	repo, err := chain.NewRepository(db, gene)
	assert.NoError(t, err)

	bestSummary := repo.BestBlockSummary()
	state := state.NewStater(db).NewState(bestSummary.Root())

	// withdraw validator3 to make it in status exit
	stakerNative := builtin.Staker.Native(state)
	increaseStakerBal(state, minStakeVET)
	err = stakerNative.AddValidation(validator3, endorser, thor.LowStakingPeriod(), minStakeVET)
	assert.NoError(t, err)

	// add delegation2 to queued validator3
	increaseStakerBal(state, minStakeVET)
	_, err = stakerNative.AddDelegation(validator3, minStakeVET, 100, 10)
	assert.NoError(t, err)

	_, err = stakerNative.WithdrawStake(validator3, endorser, 1)
	assert.NoError(t, err)
	decreaseStakerBal(state, minStakeVET)

	rt := runtime.New(
		repo.NewBestChain(),
		state,
		&xenv.BlockContext{Time: bestSummary.Header.Timestamp()},
		fc,
	)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Staker.ABI,
		to:     builtin.Staker.Address,
		caller: builtin.Staker.Address,
	}

	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(minStake).
		Caller(endorser).
		ShouldUseGas(1799).
		ShouldRevertError("StakerPaused").
		Assert(t)

	test.Case("increaseStake", validator1).
		Value(minStake).
		Caller(endorser).
		ShouldUseGas(1688).
		ShouldRevertError("StakerPaused").
		Assert(t)

	test.Case("decreaseStake", validator1, minStake).
		Caller(endorser).
		ShouldUseGas(1735).
		ShouldRevertError("StakerPaused").
		Assert(t)

	test.Case("withdrawStake", validator1).
		Caller(endorser).
		ShouldUseGas(1584).
		ShouldRevertError("StakerPaused").
		Assert(t)

	test.Case("signalExit", validator1).
		Caller(endorser).
		ShouldUseGas(1628).
		ShouldRevertError("StakerPaused").
		Assert(t)

	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		ShouldUseGas(3071).
		ShouldRevertError("StakerPaused").
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		ShouldUseGas(2755).
		ShouldRevertError("StakerPaused").
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		ShouldUseGas(2844).
		ShouldRevertError("StakerPaused").
		Assert(t)

	// change switch to pause delegator only
	builtin.Params.Native(state).Set(thor.KeyStakerSwitches, big.NewInt(0b01))

	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		ShouldUseGas(3097).
		ShouldRevertError("DelegatorPaused").
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		ShouldUseGas(2781).
		ShouldRevertError("DelegatorPaused").
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		ShouldUseGas(2870).
		ShouldRevertError("DelegatorPaused").
		Assert(t)

	// validation operations should pass
	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(minStake).
		Caller(endorser).
		ShouldUseGas(132597).
		Assert(t)

	test.Case("increaseStake", validator1).
		Value(minStake).
		Caller(endorser).
		ShouldUseGas(67215).
		Assert(t)

	test.Case("decreaseStake", validator1, minStake).
		Caller(endorser).
		ShouldUseGas(16370).
		Assert(t)

	test.Case("withdrawStake", validator1).
		Caller(endorser).
		ShouldUseGas(33226).
		Assert(t)

	// change switch to pause nothing
	builtin.Params.Native(state).Set(thor.KeyStakerSwitches, big.NewInt(0b00))

	// delegation operations should pass
	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		ShouldUseGas(48948).
		Assert(t)

	// cannot add delegation after the signal validation exit
	test.Case("signalExit", validator1).
		Caller(endorser).
		ShouldUseGas(35343).
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		ShouldUseGas(24563).
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		ShouldUseGas(21737).
		Assert(t)
}
