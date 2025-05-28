// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

type Executor struct {
	contract *bind.Caller
}

func NewExecutor(client *thorclient.Client) (*Executor, error) {
	contract, err := bind.NewCaller(client, builtin.Executor.RawABI(), builtin.Executor.Address)
	if err != nil {
		return nil, err
	}
	return &Executor{
		contract: contract,
	}, nil
}

func (e *Executor) Raw() *bind.Caller {
	return e.contract
}

func (e *Executor) Propose(signer bind.Signer, target thor.Address, data []byte) *bind.Sender {
	return e.contract.Attach(signer).Sender("propose", target, data)
}

func (e *Executor) Approve(signer bind.Signer, proposalID thor.Bytes32) *bind.Sender {
	return e.contract.Attach(signer).Sender("approve", proposalID)
}

func (e *Executor) Execute(signer bind.Signer, proposalID thor.Bytes32) *bind.Sender {
	return e.contract.Attach(signer).Sender("execute", proposalID)
}

type Proposal struct {
	ProposalID thor.Bytes32
	Action     thor.Bytes32
	Log        events.FilteredEvent
}

func (e *Executor) FilterProposals(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]Proposal, error) {
	raw, err := e.contract.FilterEvents("Proposal", eventsRange, opts, order)
	if err != nil {
		return nil, err
	}
	out := make([]Proposal, len(raw))
	for i, v := range raw {
		proposalID := *v.Topics[1]
		action := *v.Topics[2]

		out[i] = Proposal{
			ProposalID: proposalID,
			Action:     action,
			Log:        v,
		}
	}
	return out, nil
}
