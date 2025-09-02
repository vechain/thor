// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
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

	output    *[]any
	vmerr     error
	revertMsg string
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

func (c *ccase) ShouldRevert(revertMsg string) *ccase {
	c.revertMsg = revertMsg
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

	exec, _ := c.rt.PrepareClause(clause,
		0, 40000000, &xenv.TransactionContext{
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

	if c.revertMsg != "" {
		abis, err := abi.New(revertABI)
		assert.NoError(t, err)
		method, ok := abis.MethodByName("Error")
		assert.True(t, ok)
		var revertMsg string
		err = method.DecodeInput(vmout.Data, &revertMsg)
		assert.NoError(t, err)
		assert.Equal(t, c.revertMsg, revertMsg)
	}

	c.output = nil
	c.vmerr = nil
	c.events = nil
	c.revertMsg = ""

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
		ShouldRevert("staker: stake is empty").
		Assert(t)

	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(big.NewInt(1)).
		Caller(caller).
		ShouldRevert("staker: stake is not multiple of 1VET").
		Assert(t)

	test.Case("increaseStake", validation).
		Value(big.NewInt(0)).
		Caller(caller).
		ShouldRevert("staker: stake is empty").
		Assert(t)

	test.Case("increaseStake", validation).
		Value(big.NewInt(1)).
		Caller(caller).
		ShouldRevert("staker: stake is not multiple of 1VET").
		Assert(t)

	test.Case("decreaseStake", validation, big.NewInt(0)).
		Caller(caller).
		ShouldRevert("staker: stake is empty").
		Assert(t)

	test.Case("decreaseStake", validation, big.NewInt(1)).
		Caller(caller).
		ShouldRevert("staker: stake is not multiple of 1VET").
		Assert(t)

	test.Case("addDelegation", validation, uint8(100)).
		Caller(delegator).
		Value(big.NewInt(0)).
		ShouldRevert("staker: stake is empty").
		Assert(t)

	test.Case("addDelegation", validation, uint8(100)).
		Caller(delegator).
		Value(big.NewInt(1)).
		ShouldRevert("staker: stake is not multiple of 1VET").
		Assert(t)
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
		err := stakerNative.AddValidation(validator1, endorser, thor.LowStakingPeriod(), minStakeVET*2)
		if err != nil {
			return err
		}

		// add delegation1 to validator1
		_, err = stakerNative.AddDelegation(validator1, minStakeVET, 100, 10)
		if err != nil {
			return err
		}

		state.SetBalance(endorser, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))
		state.SetBalance(rich, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))
		state.SetBalance(delegator, big.NewInt(0).Mul(big.NewInt(6000e6), big.NewInt(1e18)))
		state.SetBalance(builtin.Staker.Address, big.NewInt(0).Mul(big.NewInt(50e6), big.NewInt(1e18)))

		status, err := stakerNative.SyncPOS(fc, 0)
		if err != nil {
			return err
		}
		if !status.Active {
			return errors.New("transition failed")
		}

		return nil
	})

	repo, err := chain.NewRepository(db, gene)
	assert.NoError(t, err)

	bestSummary := repo.BestBlockSummary()
	state := state.NewStater(db).NewState(bestSummary.Root())

	// withdraw validator3 to make it in status exit
	stakerNative := builtin.Staker.Native(state)
	err = stakerNative.AddValidation(validator3, endorser, thor.LowStakingPeriod(), minStakeVET)
	assert.NoError(t, err)

	// add delegation2 to queued validator3
	_, err = stakerNative.AddDelegation(validator3, minStakeVET, 100, 10)
	assert.NoError(t, err)

	_, err = stakerNative.WithdrawStake(validator3, endorser, 1)
	assert.NoError(t, err)

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
		ShouldRevert("staker: staker is paused").
		Assert(t)

	test.Case("increaseStake", validator1).
		Value(minStake).
		Caller(endorser).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	test.Case("decreaseStake", validator1, minStake).
		Caller(endorser).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	test.Case("withdrawStake", validator1).
		Caller(endorser).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	test.Case("signalExit", validator1).
		Caller(endorser).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		ShouldRevert("staker: staker is paused").
		Assert(t)

	// change switch to pause delegator only
	builtin.Params.Native(state).Set(thor.KeyStakerSwitches, big.NewInt(0b01))

	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		ShouldRevert("staker: delegator is paused").
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		ShouldRevert("staker: delegator is paused").
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		ShouldRevert("staker: delegator is paused").
		Assert(t)

	// validation operations should pass
	test.Case("addValidation", master, thor.LowStakingPeriod()).
		Value(minStake).
		Caller(endorser).
		Assert(t)

	test.Case("increaseStake", validator1).
		Value(minStake).
		Caller(endorser).
		Assert(t)

	test.Case("decreaseStake", validator1, minStake).
		Caller(endorser).
		Assert(t)

	test.Case("withdrawStake", validator1).
		Caller(endorser).
		Assert(t)

	test.Case("signalExit", validator1).
		Caller(endorser).
		Assert(t)

	// change switch to pause nothing
	builtin.Params.Native(state).Set(thor.KeyStakerSwitches, big.NewInt(0b00))

	// delegation operations should pass
	// delegation 1 to active validator1
	test.Case("addDelegation", validator1, uint8(100)).
		Value(minStake).
		Caller(delegator).
		Assert(t)

	// withdraw delegation2 on exited validator3
	test.Case("withdrawDelegation", big.NewInt(2)).
		Caller(delegator).
		Assert(t)

	// signal exit delegation1 on validator1
	test.Case("signalDelegationExit", big.NewInt(1)).
		Caller(delegator).
		Assert(t)
}
