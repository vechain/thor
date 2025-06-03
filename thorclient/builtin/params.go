// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

type Params struct {
	contract bind.Contract
}

func NewParams(client *thorclient.Client) (*Params, error) {
	contract, err := bind.NewContract(client, builtin.Params.RawABI(), &builtin.Params.Address)
	if err != nil {
		return nil, err
	}
	return &Params{
		contract: contract,
	}, nil
}

func (p *Params) Raw() bind.Contract {
	return p.contract
}

func (p *Params) Set(key thor.Bytes32, value *big.Int) bind.MethodBuilder {
	return p.contract.Method("set", key, value)
}

func (p *Params) Get(key thor.Bytes32) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.Method("get", key).Call().ExecuteInto(&out); err != nil {
		return nil, err
	}
	return out, nil
}

type SetEvent struct {
	Key   thor.Bytes32
	Value *big.Int
	Log   events.FilteredEvent
}

func (p *Params) FilterSet(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]SetEvent, error) {
	event, ok := p.contract.ABI().Events["Set"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	raw, err := p.contract.FilterEvent("Set").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	out := make([]SetEvent, len(raw))
	for i, log := range raw {
		key := log.Topics[1] // indexed key

		// non-indexed
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		out[i] = SetEvent{
			Key:   *key,
			Value: *(data[0].(**big.Int)),
			Log:   log,
		}
	}

	return out, nil
}
