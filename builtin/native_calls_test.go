// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

var errReverted = vm.ErrExecutionReverted

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
	args       []interface{}
	events     tx.Events
	provedWork *big.Int
	txID       thor.Bytes32
	blockRef   tx.BlockRef
	gasPayer   thor.Address
	expiration uint32

	output *[]interface{}
	vmerr  error
}

func (c *ctest) Case(name string, args ...interface{}) *ccase {
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

func (c *ccase) ShouldOutput(outputs ...interface{}) *ccase {
	c.output = &outputs
	return c
}

func (c *ccase) Assert(t *testing.T) *ccase {
	method, ok := c.abi.MethodByName(c.name)
	assert.True(t, ok, "should have method")

	constant := method.Const()
	stage, err := c.rt.State().Stage(0, 0)
	assert.Nil(t, err, "should stage state")
	stateRoot := stage.Hash()

	data, err := method.EncodeInput(c.args...)
	assert.Nil(t, err, "should encode input")

	exec, _ := c.rt.PrepareClause(tx.NewClause(&c.to).WithData(data),
		0, math.MaxUint64, &xenv.TransactionContext{
			ID:         c.txID,
			Origin:     c.caller,
			GasPrice:   &big.Int{},
			GasPayer:   c.gasPayer,
			ProvedWork: c.provedWork,
			BlockRef:   c.blockRef,
			Expiration: c.expiration})
	vmout, _, err := exec()
	assert.Nil(t, err)
	if constant || vmout.VMErr != nil {
		stage, err := c.rt.State().Stage(0, 0)
		assert.Nil(t, err, "should stage state")
		newStateRoot := stage.Hash()
		assert.Equal(t, stateRoot, newStateRoot)
	}
	if c.vmerr != nil {
		assert.Equal(t, c.vmerr, vmout.VMErr)
	} else {
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

	c.output = nil
	c.vmerr = nil
	c.events = nil

	return c
}

func buildGenesis(db *muxdb.MuxDB, proc func(state *state.State) error) *block.Block {
	blk, _, _, _ := new(genesis.Builder).
		Timestamp(uint64(time.Now().Unix())).
		State(proc).
		Build(state.NewStater(db))
	return blk
}

func TestParamsNative(t *testing.T) {
	executor := thor.BytesToAddress([]byte("e"))
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		return nil
	})
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{}, thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Params.ABI,
		to:  builtin.Params.Address,
	}

	key := thor.BytesToBytes32([]byte("key"))
	value := big.NewInt(999)
	setEvent := func(key thor.Bytes32, value *big.Int) *tx.Event {
		ev, _ := builtin.Params.ABI.EventByName("Set")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Params.Address,
			Topics:  []thor.Bytes32{ev.ID(), key},
			Data:    data,
		}
	}

	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	test.Case("set", key, value).
		Caller(executor).
		ShouldLog(setEvent(key, value)).
		Assert(t)

	test.Case("set", key, value).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("get", key).
		ShouldOutput(value).
		Assert(t)

}

func TestAuthorityNative(t *testing.T) {
	var (
		master1   = thor.BytesToAddress([]byte("master1"))
		endorsor1 = thor.BytesToAddress([]byte("endorsor1"))
		identity1 = thor.BytesToBytes32([]byte("identity1"))

		master2   = thor.BytesToAddress([]byte("master2"))
		endorsor2 = thor.BytesToAddress([]byte("endorsor2"))
		identity2 = thor.BytesToBytes32([]byte("identity2"))

		master3   = thor.BytesToAddress([]byte("master3"))
		endorsor3 = thor.BytesToAddress([]byte("endorsor3"))
		identity3 = thor.BytesToBytes32([]byte("identity3"))
		executor  = thor.BytesToAddress([]byte("e"))
	)

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
		state.SetBalance(thor.Address(endorsor1), thor.InitialProposerEndorsement)
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		builtin.Params.Native(state).Set(thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)
		return nil
	})
	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{}, thor.NoFork)

	candidateEvent := func(nodeMaster thor.Address, action string) *tx.Event {
		ev, _ := builtin.Authority.ABI.EventByName("Candidate")
		var b32 thor.Bytes32
		copy(b32[:], action)
		data, _ := ev.Encode(b32)
		return &tx.Event{
			Address: builtin.Authority.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(nodeMaster[:])},
			Data:    data,
		}
	}

	test := &ctest{
		rt:     rt,
		abi:    builtin.Authority.ABI,
		to:     builtin.Authority.Address,
		caller: executor,
	}

	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	test.Case("first").
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("add", master1, endorsor1, identity1).
		ShouldLog(candidateEvent(master1, "added")).
		Assert(t)

	test.Case("add", master2, endorsor2, identity2).
		ShouldLog(candidateEvent(master2, "added")).
		Assert(t)

	test.Case("add", master3, endorsor3, identity3).
		ShouldLog(candidateEvent(master3, "added")).
		Assert(t)

	test.Case("get", master1).
		ShouldOutput(true, endorsor1, identity1, true).
		Assert(t)

	test.Case("first").
		ShouldOutput(master1).
		Assert(t)

	test.Case("next", master1).
		ShouldOutput(master2).
		Assert(t)

	test.Case("next", master2).
		ShouldOutput(master3).
		Assert(t)

	test.Case("next", master3).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("add", master1, endorsor1, identity1).
		Caller(thor.BytesToAddress([]byte("other"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("add", master1, endorsor1, identity1).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("revoke", master1).
		ShouldLog(candidateEvent(master1, "revoked")).
		Assert(t)

	// duped even revoked
	test.Case("add", master1, endorsor1, identity1).
		ShouldVMError(errReverted).
		Assert(t)

	// any one can revoke a candidate if out of endorsement
	st.SetBalance(endorsor2, big.NewInt(1))
	test.Case("revoke", master2).
		Caller(thor.BytesToAddress([]byte("some one"))).
		Assert(t)

}

func TestEnergyNative(t *testing.T) {
	var (
		addr   = thor.BytesToAddress([]byte("addr"))
		to     = thor.BytesToAddress([]byte("to"))
		master = thor.BytesToAddress([]byte("master"))
		eng    = big.NewInt(1000)
	)

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		state.SetMaster(addr, master)
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(b0.Header().ID())

	st.SetEnergy(addr, eng, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, eng)

	transferEvent := func(from, to thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}
	approvalEvent := func(owner, spender thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Approval")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(owner[:]), thor.BytesToBytes32(spender[:])},
			Data:    data,
		}
	}

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, thor.NoFork)
	test := &ctest{
		rt:     rt,
		abi:    builtin.Energy.ABI,
		to:     builtin.Energy.Address,
		caller: builtin.Energy.Address,
	}

	test.Case("name").
		ShouldOutput("VeThor").
		Assert(t)

	test.Case("decimals").
		ShouldOutput(uint8(18)).
		Assert(t)

	test.Case("symbol").
		ShouldOutput("VTHO").
		Assert(t)

	test.Case("totalSupply").
		ShouldOutput(eng).
		Assert(t)

	test.Case("totalBurned").
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("balanceOf", addr).
		ShouldOutput(eng).
		Assert(t)

	test.Case("transfer", to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(transferEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("transfer", to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("move", addr, to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(transferEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("move", addr, to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("approve", to, big.NewInt(10)).
		Caller(addr).
		ShouldLog(approvalEvent(addr, to, big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("allowance", addr, to).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("transferFrom", addr, thor.BytesToAddress([]byte("some one")), big.NewInt(10)).
		Caller(to).
		ShouldLog(transferEvent(addr, thor.BytesToAddress([]byte("some one")), big.NewInt(10))).
		ShouldOutput(true).
		Assert(t)

	test.Case("transferFrom", addr, to, big.NewInt(10)).
		Caller(thor.BytesToAddress([]byte("some one"))).
		ShouldVMError(errReverted).
		Assert(t)

}

func TestPrototypeNative(t *testing.T) {
	var (
		acc1 = thor.BytesToAddress([]byte("acc1"))
		acc2 = thor.BytesToAddress([]byte("acc2"))

		master    = thor.BytesToAddress([]byte("master"))
		notmaster = thor.BytesToAddress([]byte("notmaster"))
		user      = thor.BytesToAddress([]byte("user"))
		notuser   = thor.BytesToAddress([]byte("notuser"))

		credit       = big.NewInt(1000)
		recoveryRate = big.NewInt(10)
		sponsor      = thor.BytesToAddress([]byte("sponsor"))
		notsponsor   = thor.BytesToAddress([]byte("notsponsor"))

		key      = thor.BytesToBytes32([]byte("account-key"))
		value    = thor.BytesToBytes32([]byte("account-value"))
		contract thor.Address
	)

	db := muxdb.NewMem()
	gene := genesis.NewDevnet()
	genesisBlock, _, _, _ := gene.Build(state.NewStater(db))
	repo, _ := chain.NewRepository(db, genesisBlock)
	st := state.New(db, genesisBlock.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(genesisBlock.Header().ID())

	st.SetStorage(thor.Address(acc1), key, value)
	st.SetBalance(thor.Address(acc1), big.NewInt(1))

	masterEvent := func(self, newMaster thor.Address) *tx.Event {
		ev, _ := builtin.Prototype.Events().EventByName("$Master")
		data, _ := ev.Encode(newMaster)
		return &tx.Event{
			Address: self,
			Topics:  []thor.Bytes32{ev.ID()},
			Data:    data,
		}
	}

	creditPlanEvent := func(self thor.Address, credit, recoveryRate *big.Int) *tx.Event {
		ev, _ := builtin.Prototype.Events().EventByName("$CreditPlan")
		data, _ := ev.Encode(credit, recoveryRate)
		return &tx.Event{
			Address: self,
			Topics:  []thor.Bytes32{ev.ID()},
			Data:    data,
		}
	}

	userEvent := func(self, user thor.Address, action string) *tx.Event {
		ev, _ := builtin.Prototype.Events().EventByName("$User")
		var b32 thor.Bytes32
		copy(b32[:], action)
		data, _ := ev.Encode(b32)
		return &tx.Event{
			Address: self,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(user.Bytes())},
			Data:    data,
		}
	}

	sponsorEvent := func(self, sponsor thor.Address, action string) *tx.Event {
		ev, _ := builtin.Prototype.Events().EventByName("$Sponsor")
		var b32 thor.Bytes32
		copy(b32[:], action)
		data, _ := ev.Encode(b32)
		return &tx.Event{
			Address: self,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(sponsor.Bytes())},
			Data:    data,
		}
	}

	rt := runtime.New(chain, st, &xenv.BlockContext{
		Time:   genesisBlock.Header().Timestamp(),
		Number: genesisBlock.Header().Number(),
	}, thor.NoFork)

	code, _ := hex.DecodeString("60606040523415600e57600080fd5b603580601b6000396000f3006060604052600080fd00a165627a7a72305820edd8a93b651b5aac38098767f0537d9b25433278c9d155da2135efc06927fc960029")
	exec, _ := rt.PrepareClause(tx.NewClause(nil).WithData(code), 0, math.MaxUint64, &xenv.TransactionContext{
		ID:         thor.Bytes32{},
		Origin:     master,
		GasPrice:   &big.Int{},
		ProvedWork: &big.Int{}})
	out, _, _ := exec()

	contract = *out.ContractAddress

	energy := big.NewInt(1000)
	st.SetEnergy(acc1, energy, genesisBlock.Header().Timestamp())

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.ABI,
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("master", acc1).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("master", contract).
		ShouldOutput(master).
		Assert(t)

	test.Case("setMaster", acc1, acc2).
		Caller(acc1).
		ShouldOutput().
		ShouldLog(masterEvent(acc1, acc2)).
		Assert(t)

	test.Case("setMaster", acc1, acc2).
		Caller(notmaster).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("master", acc1).
		ShouldOutput(acc2).
		Assert(t)

	test.Case("hasCode", acc1).
		ShouldOutput(false).
		Assert(t)

	test.Case("hasCode", contract).
		ShouldOutput(true).
		Assert(t)

	test.Case("setCreditPlan", contract, credit, recoveryRate).
		Caller(master).
		ShouldOutput().
		ShouldLog(creditPlanEvent(contract, credit, recoveryRate)).
		Assert(t)

	test.Case("setCreditPlan", contract, credit, recoveryRate).
		Caller(notmaster).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("creditPlan", contract).
		ShouldOutput(credit, recoveryRate).
		Assert(t)

	test.Case("isUser", contract, user).
		ShouldOutput(false).
		Assert(t)

	test.Case("addUser", contract, user).
		Caller(master).
		ShouldOutput().
		ShouldLog(userEvent(contract, user, "added")).
		Assert(t)

	test.Case("addUser", contract, user).
		Caller(notmaster).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("addUser", contract, user).
		Caller(master).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("isUser", contract, user).
		ShouldOutput(true).
		Assert(t)

	test.Case("userCredit", contract, user).
		ShouldOutput(credit).
		Assert(t)

	test.Case("removeUser", contract, user).
		Caller(master).
		ShouldOutput().
		ShouldLog(userEvent(contract, user, "removed")).
		Assert(t)

	test.Case("removeUser", contract, user).
		Caller(notmaster).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("removeUser", contract, notuser).
		Caller(master).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("userCredit", contract, user).
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	test.Case("sponsor", contract).
		Caller(sponsor).
		ShouldOutput().
		ShouldLog(sponsorEvent(contract, sponsor, "sponsored")).
		Assert(t)

	test.Case("sponsor", contract).
		Caller(sponsor).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("isSponsor", contract, sponsor).
		ShouldOutput(true).
		Assert(t)

	test.Case("currentSponsor", contract).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("selectSponsor", contract, sponsor).
		Caller(master).
		ShouldOutput().
		ShouldLog(sponsorEvent(contract, sponsor, "selected")).
		Assert(t)

	test.Case("selectSponsor", contract, notsponsor).
		Caller(master).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("selectSponsor", contract, notsponsor).
		Caller(notmaster).
		ShouldVMError(errReverted).
		Assert(t)
	test.Case("currentSponsor", contract).
		ShouldOutput(sponsor).
		Assert(t)

	test.Case("unsponsor", contract).
		Caller(sponsor).
		ShouldOutput().
		Assert(t)
	test.Case("currentSponsor", contract).
		ShouldOutput(sponsor).
		Assert(t)

	test.Case("unsponsor", contract).
		Caller(sponsor).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("isSponsor", contract, sponsor).
		ShouldOutput(false).
		Assert(t)

	test.Case("storageFor", acc1, key).
		ShouldOutput(value).
		Assert(t)
	test.Case("storageFor", acc1, thor.BytesToBytes32([]byte("some-key"))).
		ShouldOutput(thor.Bytes32{}).
		Assert(t)

	// should be hash of rlp raw
	expected, err := st.GetStorage(builtin.Prototype.Address, thor.Blake2b(contract.Bytes(), []byte("credit-plan")))
	assert.Nil(t, err)
	test.Case("storageFor", builtin.Prototype.Address, thor.Blake2b(contract.Bytes(), []byte("credit-plan"))).
		ShouldOutput(expected).
		Assert(t)

	test.Case("balance", acc1, big.NewInt(0)).
		ShouldOutput(big.NewInt(1)).
		Assert(t)

	test.Case("balance", acc1, big.NewInt(100)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(0)).
		ShouldOutput(energy).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(100)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)
	{
		hash, _ := st.GetCodeHash(builtin.Prototype.Address)
		assert.False(t, hash.IsZero())
	}
}

func TestPrototypeNativeWithLongerBlockNumber(t *testing.T) {
	var (
		acc1 = thor.BytesToAddress([]byte("acc1"))
		sig  [65]byte
	)
	rand.Read(sig[:])

	db := muxdb.NewMem()
	gene := genesis.NewDevnet()
	genesisBlock, _, _, _ := gene.Build(state.NewStater(db))
	st := state.New(db, genesisBlock.Header().StateRoot(), 0, 0, 0)
	repo, _ := chain.NewRepository(db, genesisBlock)
	launchTime := genesisBlock.Header().Timestamp()

	for i := 1; i < 100; i++ {
		st.SetBalance(acc1, big.NewInt(int64(i)))
		st.SetEnergy(acc1, big.NewInt(int64(i)), launchTime+uint64(i)*10)
		stage, _ := st.Stage(uint32(i), 0)
		stateRoot, _ := stage.Commit()
		b := new(block.Builder).
			ParentID(repo.BestBlockSummary().Header.ID()).
			TotalScore(repo.BestBlockSummary().Header.TotalScore() + 1).
			Timestamp(launchTime + uint64(i)*10).
			StateRoot(stateRoot).
			Build().
			WithSignature(sig[:])
		repo.AddBlock(b, tx.Receipts{}, 0)
		repo.SetBestBlockID(b.Header().ID())
	}

	st = state.New(db, repo.BestBlockSummary().Header.StateRoot(), repo.BestBlockSummary().Header.Number(), 0, 0)
	chain := repo.NewBestChain()

	rt := runtime.New(chain, st, &xenv.BlockContext{
		Number: thor.MaxStateHistory + 1,
		Time:   repo.BestBlockSummary().Header.Timestamp(),
	}, thor.NoFork)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.ABI,
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("balance", acc1, big.NewInt(0)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(0)).
		ShouldOutput(big.NewInt(0)).
		Assert(t)

	test.Case("balance", acc1, big.NewInt(1)).
		ShouldOutput(big.NewInt(1)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(1)).
		ShouldOutput(big.NewInt(1)).
		Assert(t)

	test.Case("balance", acc1, big.NewInt(2)).
		ShouldOutput(big.NewInt(2)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(2)).
		ShouldOutput(big.NewInt(2)).
		Assert(t)
}

func TestPrototypeNativeWithBlockNumber(t *testing.T) {
	var (
		acc1 = thor.BytesToAddress([]byte("acc1"))
		sig  [65]byte
	)
	rand.Read(sig[:])

	db := muxdb.NewMem()
	gene := genesis.NewDevnet()
	genesisBlock, _, _, _ := gene.Build(state.NewStater(db))
	st := state.New(db, genesisBlock.Header().StateRoot(), 0, 0, 0)
	repo, _ := chain.NewRepository(db, genesisBlock)
	launchTime := genesisBlock.Header().Timestamp()

	for i := 1; i < 100; i++ {
		st.SetBalance(acc1, big.NewInt(int64(i)))
		st.SetEnergy(acc1, big.NewInt(int64(i)), launchTime+uint64(i)*10)
		stage, _ := st.Stage(uint32(i), 0)
		stateRoot, _ := stage.Commit()
		b := new(block.Builder).
			ParentID(repo.BestBlockSummary().Header.ID()).
			TotalScore(repo.BestBlockSummary().Header.TotalScore() + 1).
			Timestamp(launchTime + uint64(i)*10).
			StateRoot(stateRoot).
			Build().
			WithSignature(sig[:])
		repo.AddBlock(b, tx.Receipts{}, 0)
		repo.SetBestBlockID(b.Header().ID())
	}

	st = state.New(db, repo.BestBlockSummary().Header.StateRoot(), repo.BestBlockSummary().Header.Number(), 0, repo.BestBlockSummary().SteadyNum)
	chain := repo.NewBestChain()

	rt := runtime.New(chain, st, &xenv.BlockContext{
		Number: repo.BestBlockSummary().Header.Number(),
		Time:   repo.BestBlockSummary().Header.Timestamp(),
	}, thor.NoFork)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Prototype.ABI,
		to:     builtin.Prototype.Address,
		caller: builtin.Prototype.Address,
	}

	test.Case("balance", acc1, big.NewInt(10)).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(10)).
		ShouldOutput(big.NewInt(10)).
		Assert(t)

	test.Case("balance", acc1, big.NewInt(99)).
		ShouldOutput(big.NewInt(99)).
		Assert(t)

	test.Case("energy", acc1, big.NewInt(99)).
		ShouldOutput(big.NewInt(99)).
		Assert(t)
}

func newBlock(parent *block.Block, score uint64, timestamp uint64, privateKey *ecdsa.PrivateKey) *block.Block {
	b := new(block.Builder).ParentID(parent.Header().ID()).TotalScore(parent.Header().TotalScore() + score).Timestamp(timestamp).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}

func TestExtensionNative(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, thor.Bytes32{}, 0, 0, 0)
	gene := genesis.NewDevnet()
	genesisBlock, _, _, _ := gene.Build(state.NewStater(db))
	repo, _ := chain.NewRepository(db, genesisBlock)
	st.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes())

	privKeys := make([]*ecdsa.PrivateKey, 2)

	for i := 0; i < 2; i++ {
		privateKey, _ := crypto.GenerateKey()
		privKeys[i] = privateKey
	}

	b0 := genesisBlock
	b1 := newBlock(b0, 123, 456, privKeys[0])
	b2 := newBlock(b1, 789, 321, privKeys[1])

	b1_singer, _ := b1.Header().Signer()
	b2_singer, _ := b2.Header().Signer()

	gasPayer := thor.BytesToAddress([]byte("gasPayer"))

	err := repo.AddBlock(b1, nil, 0)
	assert.Equal(t, err, nil)
	err = repo.AddBlock(b2, nil, 0)
	assert.Equal(t, err, nil)

	assert.Equal(t, builtin.Extension.Address, builtin.Extension.Address)

	chain := repo.NewChain(b2.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{Number: 2, Time: b2.Header().Timestamp(), TotalScore: b2.Header().TotalScore(), Signer: b2_singer}, thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Extension.V2.ABI,
		to:  builtin.Extension.Address,
	}

	test.Case("blake2b256", []byte("hello world")).
		ShouldOutput(thor.Blake2b([]byte("hello world"))).
		Assert(t)

	expected, _ := builtin.Energy.Native(st, 0).TokenTotalSupply()
	test.Case("totalSupply").
		ShouldOutput(expected).
		Assert(t)

	test.Case("txBlockRef").
		BlockRef(tx.NewBlockRef(1)).
		ShouldOutput(tx.NewBlockRef(1)).
		Assert(t)

	test.Case("txExpiration").
		Expiration(100).
		ShouldOutput(big.NewInt(100)).
		Assert(t)

	test.Case("txProvedWork").
		ProvedWork(big.NewInt(1e12)).
		ShouldOutput(big.NewInt(1e12)).
		Assert(t)

	test.Case("txID").
		TxID(thor.BytesToBytes32([]byte("txID"))).
		ShouldOutput(thor.BytesToBytes32([]byte("txID"))).
		Assert(t)

	test.Case("blockID", big.NewInt(3)).
		ShouldOutput(thor.Bytes32{}).
		Assert(t)

	test.Case("blockID", big.NewInt(2)).
		ShouldOutput(thor.Bytes32{}).
		Assert(t)

	test.Case("blockID", big.NewInt(1)).
		ShouldOutput(b1.Header().ID()).
		Assert(t)

	test.Case("blockID", big.NewInt(0)).
		ShouldOutput(b0.Header().ID()).
		Assert(t)

	test.Case("blockTotalScore", big.NewInt(3)).
		ShouldOutput(uint64(0)).
		Assert(t)

	test.Case("blockTotalScore", big.NewInt(2)).
		ShouldOutput(b2.Header().TotalScore()).
		Assert(t)

	test.Case("blockTotalScore", big.NewInt(1)).
		ShouldOutput(b1.Header().TotalScore()).
		Assert(t)

	test.Case("blockTotalScore", big.NewInt(0)).
		ShouldOutput(b0.Header().TotalScore()).
		Assert(t)

	test.Case("blockTime", big.NewInt(3)).
		ShouldOutput(&big.Int{}).
		Assert(t)

	test.Case("blockTime", big.NewInt(2)).
		ShouldOutput(new(big.Int).SetUint64(b2.Header().Timestamp())).
		Assert(t)

	test.Case("blockTime", big.NewInt(1)).
		ShouldOutput(new(big.Int).SetUint64(b1.Header().Timestamp())).
		Assert(t)

	test.Case("blockTime", big.NewInt(0)).
		ShouldOutput(new(big.Int).SetUint64(b0.Header().Timestamp())).
		Assert(t)

	test.Case("blockSigner", big.NewInt(3)).
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("blockSigner", big.NewInt(2)).
		ShouldOutput(b2_singer).
		Assert(t)

	test.Case("blockSigner", big.NewInt(1)).
		ShouldOutput(b1_singer).
		Assert(t)

	test.Case("txGasPayer").
		ShouldOutput(thor.Address{}).
		Assert(t)

	test.Case("txGasPayer").
		GasPayer(gasPayer).
		ShouldOutput(gasPayer).
		Assert(t)

}
