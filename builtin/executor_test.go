// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

func M(a ...interface{}) []interface{} {
	return a
}

func approverEvent(approver thor.Address, action string) *tx.Event {
	ev, _ := builtin.Executor.ABI.EventByName("Approver")
	var b32 thor.Bytes32
	copy(b32[:], action)
	data, _ := ev.Encode(b32)
	return &tx.Event{
		Address: builtin.Executor.Address,
		Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(approver.Bytes())},
		Data:    data,
	}
}
func proposalEvent(id thor.Bytes32, action string) *tx.Event {
	ev, _ := builtin.Executor.ABI.EventByName("Proposal")
	var b32 thor.Bytes32
	copy(b32[:], action)
	data, _ := ev.Encode(b32)
	return &tx.Event{
		Address: builtin.Executor.Address,
		Topics:  []thor.Bytes32{ev.ID(), id},
		Data:    data,
	}
}

func votingContractEvent(addr thor.Address, action string) *tx.Event {
	ev, _ := builtin.Executor.ABI.EventByName("VotingContract")
	var b32 thor.Bytes32
	copy(b32[:], action)
	data, _ := ev.Encode(b32)
	return &tx.Event{
		Address: builtin.Executor.Address,
		Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(addr.Bytes())},
		Data:    data,
	}
}

func initExectorTest() *ctest {
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes())
		state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(builtin.Executor.Address[:]))
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: uint64(time.Now().Unix())}, thor.NoFork)

	return &ctest{
		rt:  rt,
		abi: builtin.Executor.ABI,
		to:  builtin.Executor.Address,
	}
}

func TestExecutorApprover(t *testing.T) {
	test := initExectorTest()
	var approvers []thor.Address
	for i := 0; i < 7; i++ {
		approvers = append(approvers, thor.BytesToAddress([]byte(fmt.Sprintf("approver%d", i))))
	}

	for _, a := range approvers {
		// zero identity
		test.Case("addApprover", a, thor.Bytes32{}).
			ShouldVMError(errReverted).
			Assert(t)

		test.Case("addApprover", a, thor.BytesToBytes32(a.Bytes())).
			Caller(thor.BytesToAddress([]byte("other"))).
			ShouldVMError(errReverted).
			Assert(t)

		test.Case("addApprover", a, thor.BytesToBytes32(a.Bytes())).
			Caller(builtin.Executor.Address).
			ShouldLog(approverEvent(a, "added")).
			Assert(t)
		assert.Equal(t, M(true, nil), M(builtin.Prototype.Native(test.rt.State()).Bind(test.to).IsUser(a)))
	}

	test.Case("approverCount").
		ShouldOutput(uint8(len(approvers))).
		Assert(t)

	test.Case("addApprover", approvers[0], thor.BytesToBytes32(approvers[0].Bytes())).
		Caller(builtin.Executor.Address).
		ShouldVMError(errReverted).
		Assert(t)

	for _, a := range approvers {
		test.Case("approvers", a).
			ShouldOutput(thor.BytesToBytes32(a.Bytes()), true).
			Assert(t)
	}

	for _, a := range approvers {
		test.Case("revokeApprover", a).
			ShouldVMError(errReverted).
			Assert(t)

		test.Case("revokeApprover", a).
			Caller(builtin.Executor.Address).
			ShouldLog(approverEvent(a, "revoked")).
			Assert(t)
		assert.Equal(t, M(false, nil), M(builtin.Prototype.Native(test.rt.State()).Bind(test.to).IsUser(a)))
	}
	test.Case("approverCount").
		ShouldOutput(uint8(0)).
		Assert(t)
}

func TestExecutorVotingContract(t *testing.T) {

	test := initExectorTest()
	voting := thor.BytesToAddress([]byte("voting"))
	test.Case("attachVotingContract", voting).
		ShouldVMError(errReverted).
		Assert(t)
	test.Case("votingContracts", voting).
		ShouldOutput(false).
		Assert(t)
	test.Case("attachVotingContract", voting).
		Caller(builtin.Executor.Address).
		ShouldLog(votingContractEvent(voting, "attached")).
		Assert(t)

	test.Case("votingContracts", voting).
		ShouldOutput(true).
		Assert(t)

	test.Case("attachVotingContract", voting).
		Caller(builtin.Executor.Address).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("detachVotingContract", voting).
		Caller(builtin.Executor.Address).
		ShouldLog(votingContractEvent(voting, "detached")).
		Assert(t)

	test.Case("attachVotingContract", voting).
		Caller(builtin.Executor.Address).
		ShouldLog(votingContractEvent(voting, "attached")).
		Assert(t)
}

func TestExecutorProposal(t *testing.T) {
	test := initExectorTest()

	target := builtin.Params.Address
	setParam, _ := builtin.Params.ABI.MethodByName("set")
	data, _ := setParam.EncodeInput(thor.BytesToBytes32([]byte("paramKey")), big.NewInt(12345))
	test.Case("propose", target, data).
		ShouldVMError(errReverted).
		Assert(t)

	approver := thor.BytesToAddress([]byte("approver"))
	test.Case("addApprover", approver, thor.BytesToBytes32(approver.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)

	proposalID := func() thor.Bytes32 {
		var b8 [8]byte
		binary.BigEndian.PutUint64(b8[:], test.rt.Context().Time)
		return thor.Bytes32(crypto.Keccak256Hash(b8[:], approver[:]))
	}()
	test.Case("propose", target, data).
		Caller(approver).
		ShouldOutput(proposalID).
		ShouldLog(proposalEvent(proposalID, "proposed")).
		Assert(t)

	test.Case("proposals", proposalID).
		ShouldOutput(
			test.rt.Context().Time,
			approver,
			uint8(1),
			uint8(0),
			false,
			target,
			data).
		Assert(t)

	test.Case("approve", proposalID).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("execute", proposalID).
		ShouldVMError(errReverted).
		Assert(t)

	test.Case("approve", proposalID).
		Caller(approver).
		ShouldLog(proposalEvent(proposalID, "approved")).
		Assert(t)
	test.Case("proposals", proposalID).
		ShouldOutput(
			test.rt.Context().Time,
			approver,
			uint8(1),
			uint8(1),
			false,
			target,
			data).
		Assert(t)

	test.Case("execute", proposalID).
		ShouldLog(proposalEvent(proposalID, "executed")).
		Assert(t)

	test.Case("execute", proposalID).
		ShouldVMError(errReverted).
		Assert(t)
	test.Case("proposals", proposalID).
		ShouldOutput(
			test.rt.Context().Time,
			approver,
			uint8(1),
			uint8(1),
			true,
			target,
			data).
		Assert(t)

	assert.Equal(t, M(big.NewInt(12345), nil), M(builtin.Params.Native(test.rt.State()).Get(thor.BytesToBytes32([]byte("paramKey")))))
}
