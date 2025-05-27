package bind

import (
	_ "embed"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// Energy is a type-safe smart contract wrapper of VTHO.
type Energy struct {
	caller *Caller
	client *thorclient.Client
}

func NewEnergy(client *thorclient.Client) *Energy {
	caller, err := NewCaller(client, builtin.Energy.RawABI(), builtin.Energy.Address)
	if err != nil {
		panic(err)
	}
	return &Energy{
		caller: caller,
		client: client,
	}
}

func (e *Energy) Revision(blockID string) *Energy {
	return &Energy{
		caller: e.caller.Revision(blockID),
		client: e.client,
	}
}

// Name returns the name of the token
func (e *Energy) Name() (string, error) {
	var name string
	if err := e.caller.CallInto("name", &name); err != nil {
		return "", err
	}
	return name, nil
}

// Symbol returns the symbol of the token
func (e *Energy) Symbol() (string, error) {
	var symbol string
	if err := e.caller.CallInto("symbol", &symbol); err != nil {
		return "", err
	}
	return symbol, nil
}

// Decimals returns the number of decimals the token uses
func (e *Energy) Decimals() (uint8, error) {
	var decimals uint8
	if err := e.caller.CallInto("decimals", &decimals); err != nil {
		return 0, err
	}
	return decimals, nil
}

// TotalSupply returns the total token supply
func (e *Energy) TotalSupply() (*big.Int, error) {
	out := new(big.Int)
	if err := e.caller.CallInto("totalSupply", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TotalBurned returns the total amount of burned tokens
func (e *Energy) TotalBurned() (*big.Int, error) {
	out := new(big.Int)
	if err := e.caller.CallInto("totalBurned", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BalanceOf returns the token balance of the specified address
func (e *Energy) BalanceOf(owner thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := e.caller.CallInto("balanceOf", &out, owner); err != nil {
		return nil, err
	}
	return out, nil
}

// Allowance returns the amount of tokens approved by the owner to be spent by the spender
func (e *Energy) Allowance(owner, spender thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := e.caller.CallInto("allowance", &out, owner, spender); err != nil {
		return nil, err
	}
	return out, nil
}

// Transfer transfers tokens to the specified address
func (e *Energy) Transfer(signer Signer, to thor.Address, amount *big.Int) *Sender {
	return e.caller.Attach(signer).Sender("transfer", to, amount)
}

// TransferFrom transfers tokens from one address to another
func (e *Energy) TransferFrom(signer Signer, from, to thor.Address, amount *big.Int) *Sender {
	return e.caller.Attach(signer).Sender("transferFrom", from, to, amount)
}

// Approve approves the spender to spend the specified amount of tokens
func (e *Energy) Approve(signer Signer, spender thor.Address, amount *big.Int) *Sender {
	return e.caller.Attach(signer).Sender("approve", spender, amount)
}

// Move transfers tokens from one address to another (alias for transferFrom)
func (e *Energy) Move(signer Signer, from, to thor.Address, amount *big.Int) *Sender {
	return e.caller.Attach(signer).Sender("move", from, to, amount)
}

// TransferEvent represents the Transfer event
type TransferEvent struct {
	From  thor.Address
	To    thor.Address
	Value *big.Int
}

// FilterTransfer filters Transfer events for the specified range and options.
func (e *Energy) FilterTransfer(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]TransferEvent, error) {
	event, ok := e.caller.ABI().Events["Transfer"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	logs, err := e.caller.FilterEvents("Transfer", eventsRange, opts, order)
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

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		events[i] = TransferEvent{
			From:  fromAddr,
			To:    toAddr,
			Value: *(data[0].(**big.Int)),
		}
	}

	return events, nil
}

// ApprovalEvent represents the Approval event
type ApprovalEvent struct {
	Owner   thor.Address
	Spender thor.Address
	Value   *big.Int
}

// FilterApproval filters Approval events for the specified range and options.
func (e *Energy) FilterApproval(eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]ApprovalEvent, error) {
	event, ok := e.caller.ABI().Events["Approval"]
	if !ok {
		return nil, fmt.Errorf("event not found")
	}

	logs, err := e.caller.FilterEvents("Approval", eventsRange, opts, order)
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

		if err := event.Inputs.Unpack(&data, bytes); err != nil {
			return nil, err
		}

		events[i] = ApprovalEvent{
			Owner:   ownerAddr,
			Spender: spenderAddr,
			Value:   *(data[0].(**big.Int)),
		}
	}

	return events, nil
}
