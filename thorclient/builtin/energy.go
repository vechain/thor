// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	_ "embed"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/abi"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

// Energy is a type-safe smart contract wrapper of VTHO.
type Energy struct {
	contract *bind.Contract
	revision string
}

func NewEnergy(client *thorclient.Client) (*Energy, error) {
	contract, err := bind.NewContract(client, builtin.Energy.RawABI(), &builtin.Energy.Address)
	if err != nil {
		return nil, err
	}
	return &Energy{
		contract: contract,
	}, nil
}

// Revision creates a new Energy instance with the specified revision.
func (e *Energy) Revision(rev string) *Energy {
	return &Energy{
		contract: e.contract,
		revision: rev,
	}
}

func (e *Energy) Raw() *bind.Contract {
	return e.contract
}

// Name returns the name of the token
func (e *Energy) Name() (string, error) {
	var name string
	if err := e.contract.Method("name").Call().AtRevision(e.revision).ExecuteInto(&name); err != nil {
		return "", err
	}
	return name, nil
}

// Symbol returns the symbol of the token
func (e *Energy) Symbol() (string, error) {
	var symbol string
	if err := e.contract.Method("symbol").Call().AtRevision(e.revision).ExecuteInto(&symbol); err != nil {
		return "", err
	}
	return symbol, nil
}

// Decimals returns the number of decimals the token uses
func (e *Energy) Decimals() (uint8, error) {
	var decimals uint8
	if err := e.contract.Method("decimals").Call().AtRevision(e.revision).ExecuteInto(&decimals); err != nil {
		return 0, err
	}
	return decimals, nil
}

// TotalSupply returns the total token supply
func (e *Energy) TotalSupply() (*big.Int, error) {
	totalSupply := new(big.Int)
	if err := e.contract.Method("totalSupply").Call().AtRevision(e.revision).ExecuteInto(&totalSupply); err != nil {
		return nil, err
	}
	return totalSupply, nil
}

// TotalBurned returns the total amount of burned tokens
func (e *Energy) TotalBurned() (*big.Int, error) {
	totalBurned := new(big.Int)
	if err := e.contract.Method("totalBurned").Call().AtRevision(e.revision).ExecuteInto(&totalBurned); err != nil {
		return nil, err
	}
	return totalBurned, nil
}

// BalanceOf returns the token balance of the specified address
func (e *Energy) BalanceOf(owner thor.Address) (*big.Int, error) {
	balanceOf := new(big.Int)
	if err := e.contract.Method("balanceOf", owner).Call().AtRevision(e.revision).ExecuteInto(&balanceOf); err != nil {
		return nil, err
	}
	return balanceOf, nil
}

// Allowance returns the amount of tokens approved by the owner to be spent by the spender
func (e *Energy) Allowance(owner, spender thor.Address) (*big.Int, error) {
	allowance := new(big.Int)
	if err := e.contract.Method("allowance", owner, spender).Call().AtRevision(e.revision).ExecuteInto(&allowance); err != nil {
		return nil, err
	}
	return allowance, nil
}

// Transfer transfers tokens to the specified address
func (e *Energy) Transfer(to thor.Address, amount *big.Int) *bind.MethodBuilder {
	return e.contract.Method("transfer", to, amount)
}

// TransferFrom transfers tokens from one address to another
func (e *Energy) TransferFrom(from, to thor.Address, amount *big.Int) *bind.MethodBuilder {
	return e.contract.Method("transferFrom", from, to, amount)
}

// Approve approves the spender to spend the specified amount of tokens
func (e *Energy) Approve(spender thor.Address, amount *big.Int) *bind.MethodBuilder {
	return e.contract.Method("approve", spender, amount)
}

// Move transfers tokens from one address to another (alias for transferFrom)
func (e *Energy) Move(from, to thor.Address, amount *big.Int) *bind.MethodBuilder {
	return e.contract.Method("move", from, to, amount)
}

// TransferEvent represents the Transfer event
type TransferEvent struct {
	From  thor.Address
	To    thor.Address
	Value *big.Int
	Log   api.FilteredEvent
}

// FilterTransfer filters Transfer events for the specified range and options.
func (e *Energy) FilterTransfer(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]TransferEvent, error) {
	event, ok := e.contract.ABI().Events["Transfer"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	logs, err := e.contract.FilterEvent("Transfer").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	events := make([]TransferEvent, len(logs))
	for i, log := range logs {
		fromAddr := thor.BytesToAddress(log.Topics[1][:])
		toAddr := thor.BytesToAddress(log.Topics[2][:])

		// Non-indexed fields
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := abi.UnpackIntoInterface(&event.Inputs, bytes, &data); err != nil {
			return nil, err
		}

		events[i] = TransferEvent{
			From:  fromAddr,
			To:    toAddr,
			Value: *(data[0].(**big.Int)),
			Log:   log,
		}
	}

	return events, nil
}

// ApprovalEvent represents the Approval event
type ApprovalEvent struct {
	Owner   thor.Address
	Spender thor.Address
	Value   *big.Int
	Log     api.FilteredEvent
}

// FilterApproval filters Approval events for the specified range and options.
func (e *Energy) FilterApproval(eventsRange *api.Range, opts *api.Options, order logdb.Order) ([]ApprovalEvent, error) {
	event, ok := e.contract.ABI().Events["Approval"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	logs, err := e.contract.FilterEvent("Approval").WithOptions(opts).InRange(eventsRange).OrderBy(order).Execute()
	if err != nil {
		return nil, err
	}

	events := make([]ApprovalEvent, len(logs))
	for i, log := range logs {
		ownerAddr := thor.BytesToAddress(log.Topics[1][:])
		spenderAddr := thor.BytesToAddress(log.Topics[2][:])

		// Non-indexed fields
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(log.Data)
		if err != nil {
			return nil, err
		}

		if err := abi.UnpackIntoInterface(&event.Inputs, bytes, &data); err != nil {
			return nil, err
		}

		events[i] = ApprovalEvent{
			Owner:   ownerAddr,
			Spender: spenderAddr,
			Value:   *(data[0].(**big.Int)),
			Log:     log,
		}
	}

	return events, nil
}
