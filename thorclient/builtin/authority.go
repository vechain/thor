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
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

type Authority struct {
	contract *bind.Caller
}

func NewAuthority(client *thorclient.Client) (*Authority, error) {
	base, err := bind.NewCaller(client, builtin.Authority.RawABI(), builtin.Authority.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to create authority contract: %w", err)
	}
	return &Authority{
		contract: base,
	}, nil
}

func (a *Authority) Raw() *bind.Caller {
	return a.contract
}

func (a *Authority) Revision(blockID string) *Authority {
	return &Authority{
		contract: a.contract.Revision(blockID),
	}
}

// First returns the first authority node
func (a *Authority) First() (thor.Address, error) {
	out := new(common.Address)
	if err := a.contract.CallInto("first", &out); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// Next returns the next authority node after the given node master
func (a *Authority) Next(nodeMaster thor.Address) (thor.Address, error) {
	out := new(common.Address)
	if err := a.contract.CallInto("next", &out, common.Address(nodeMaster)); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// Executor returns the executor address
func (a *Authority) Executor() (thor.Address, error) {
	out := new(common.Address)
	if err := a.contract.CallInto("executor", &out); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

type AuthorityNode struct {
	Listed   bool
	Endorsor thor.Address
	Identity thor.Bytes32
	Active   bool
}

// Get returns the authority node information for the given node master
func (a *Authority) Get(nodeMaster thor.Address) (*AuthorityNode, error) {
	var out = [4]any{}
	out[0] = new(bool)
	out[1] = new(common.Address)
	out[2] = new(common.Hash)
	out[3] = new(bool)

	if err := a.contract.CallInto("get", &out, common.Address(nodeMaster)); err != nil {
		return nil, err
	}

	node := &AuthorityNode{
		Listed:   *(out[0].(*bool)),
		Endorsor: thor.Address(*(out[1].(*common.Address))),
		Identity: thor.Bytes32(*(out[2].(*common.Hash))),
		Active:   *(out[3].(*bool)),
	}

	return node, nil
}

// Add adds a new authority node
func (a *Authority) Add(signer bind.Signer, nodeMaster, endorsor thor.Address, identity thor.Bytes32) *bind.Sender {
	return a.contract.Attach(signer).Sender("add", common.Address(nodeMaster), common.Address(endorsor), common.Hash(identity))
}

// Revoke revokes an authority node
func (a *Authority) Revoke(signer bind.Signer, nodeMaster thor.Address) *bind.Sender {
	return a.contract.Attach(signer).Sender("revoke", common.Address(nodeMaster))
}

type CandidateEvent struct {
	NodeMaster thor.Address
	Action     thor.Bytes32
	Log        events.FilteredEvent
}

// FilterCandidate filters Candidate events within the given block range
func (a *Authority) FilterCandidate(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]CandidateEvent, error) {
	event, ok := a.contract.ABI().Events["Candidate"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := a.contract.FilterEvents("Candidate", eventsRange, opts, order)
	if err != nil {
		return nil, err
	}

	out := make([]CandidateEvent, len(raw))
	for i, log := range raw {
		nodeMaster := thor.BytesToAddress(log.Topics[1][:]) // indexed

		// non-indexed data
		data := make([]any, 1)
		data[0] = new(common.Hash)

		bytes := common.FromHex(log.Data)
		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = CandidateEvent{
			NodeMaster: nodeMaster,
			Action:     thor.Bytes32(*(data[0].(*common.Hash))),
			Log:        log,
		}
	}

	return out, nil
}
