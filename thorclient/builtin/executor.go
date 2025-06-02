// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/thorclient/httpclient"
)

type Executor struct {
	contract bind.Contract
}

func NewExecutor(client *httpclient.Client) (*Executor, error) {
	contract, err := bind.NewContract(client, builtin.Executor.RawABI(), &builtin.Executor.Address)
	if err != nil {
		return nil, err
	}
	return &Executor{
		contract: contract,
	}, nil
}

func (e *Executor) Raw() bind.Contract {
	return e.contract
}

type Approver struct {
	Identity thor.Bytes32
	InPower  bool
}

func (e *Executor) Approvers(address thor.Address) (*Approver, error) {
	out := make([]any, 2)
	out[0] = new(common.Hash)
	out[1] = new(bool)

	if err := e.contract.Method("approvers", address).Call().Into(&out); err != nil {
		return nil, fmt.Errorf("failed to call approvers: %w", err)
	}

	return &Approver{
		Identity: thor.Bytes32(*out[0].(*common.Hash)),
		InPower:  *out[1].(*bool),
	}, nil
}

type Proposal struct {
	TimeProposed  uint64
	Proposer      thor.Address
	Quorum        uint8
	ApprovalCount uint8
	Executed      bool
	Target        thor.Address
	Data          []byte
}

func (e *Executor) Proposals(proposalID thor.Bytes32) (*Proposal, error) {
	out := make([]any, 7)
	out[0] = new(uint64)
	out[1] = new(common.Address)
	out[2] = new(uint8)
	out[3] = new(uint8)
	out[4] = new(bool)
	out[5] = new(common.Address)
	out[6] = new([]byte)

	if err := e.contract.Method("proposals", proposalID).Call().Into(&out); err != nil {
		return nil, fmt.Errorf("failed to call proposals: %w", err)
	}

	return &Proposal{
		TimeProposed:  *out[0].(*uint64),
		Proposer:      (thor.Address)(*out[1].(*common.Address)),
		Quorum:        *out[2].(*uint8),
		ApprovalCount: *out[3].(*uint8),
		Executed:      *out[4].(*bool),
		Target:        (thor.Address)(*out[5].(*common.Address)),
		Data:          *out[6].(*[]byte),
	}, nil
}

func (e *Executor) ApproverCount() (uint8, error) {
	var count uint8
	if err := e.contract.Method("approverCount").Call().Into(&count); err != nil {
		return 0, fmt.Errorf("failed to call approverCount: %w", err)
	}
	return count, nil
}

func (e *Executor) Propose(target thor.Address, data []byte) bind.MethodBuilder {
	return e.contract.Method("propose", target, data)
}

func (e *Executor) Approve(proposalID thor.Bytes32) bind.MethodBuilder {
	return e.contract.Method("approve", proposalID)
}

func (e *Executor) Execute(proposalID thor.Bytes32) bind.MethodBuilder {
	return e.contract.Method("execute", proposalID)
}

func (e *Executor) AddApprover(address thor.Address, identity thor.Bytes32) bind.MethodBuilder {
	return e.contract.Method("addApprover", address, identity)
}

func (e *Executor) RevokeApprover(address thor.Address) bind.MethodBuilder {
	return e.contract.Method("revokeApprover", address)
}

func (e *Executor) AttachVotingContract(votingContract thor.Address) bind.MethodBuilder {
	return e.contract.Method("attachVotingContract", votingContract)
}

func (e *Executor) DetachVotingContract(votingContract thor.Address) bind.MethodBuilder {
	return e.contract.Method("detachVotingContract", votingContract)
}

type ProposalEvent struct {
	ProposalID thor.Bytes32
	Action     string
	Log        events.FilteredEvent
}

func (e *Executor) FilterProposals(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]ProposalEvent, error) {
	_, ok := e.contract.ABI().Events["Proposal"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := e.contract.FilterEvent("Proposal").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}
	out := make([]ProposalEvent, len(raw))
	for i, v := range raw {
		out[i] = ProposalEvent{
			ProposalID: *v.Topics[1],
			Action:     v.Data,
			Log:        v,
		}
	}
	return out, nil
}
