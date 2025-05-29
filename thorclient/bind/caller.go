// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thor/v2/tx"
)

// Caller is a generic contract wrapper that allows calling methods and filtering events.
type Caller struct {
	client *httpclient.Client
	abi    *abi.ABI
	addr   thor.Address
	rev    *string
}

func NewCaller(client *httpclient.Client, abiData []byte, address thor.Address) (*Caller, error) {
	contractABI, err := abi.JSON(bytes.NewReader(abiData))
	if err != nil {
		return nil, err
	}
	return &Caller{
		client: client,
		abi:    &contractABI,
		addr:   address,
	}, nil
}

func (w *Caller) Address() thor.Address {
	return w.addr
}

func (w *Caller) ABI() *abi.ABI {
	return w.abi
}

// Revision creates a new instance and sets the revision for the contract wrapper. Allows querying historical states.
func (w *Caller) Revision(rev string) *Caller {
	return &Caller{
		client: w.client,
		abi:    w.abi,
		addr:   w.addr,
		rev:    &rev,
	}
}

// Attach creates a new Transactor instance with the provided signer.
func (w *Caller) Attach(signer Signer) *Transactor {
	return NewTransactor(signer, w)
}

// Client returns the underlying HTTP client used by the Caller.
func (w *Caller) Client() *httpclient.Client {
	return w.client
}

// Call a method and return the result as a CallResult.
func (w *Caller) Call(methodName string, args ...any) (*accounts.CallResult, error) {
	return w.Simulate(big.NewInt(0), thor.Address{}, methodName, args...)
}

// Simulate a contract call with the specified VET value and caller address.
// It can be used to estimate gas or check the result of a call without sending a transaction.
func (w *Caller) Simulate(vet *big.Int, caller thor.Address, methodName string, args ...any) (*accounts.CallResult, error) {
	clause, err := w.ClauseWithVET(vet, methodName, args...)
	if err != nil {
		return nil, err
	}
	body := &accounts.BatchCallData{
		Caller: &caller,
		Clauses: []accounts.Clause{
			{
				To:    &w.addr,
				Data:  hexutil.Encode(clause.Data()),
				Value: (*math.HexOrDecimal256)(clause.Value()),
			},
		},
	}
	var res []*accounts.CallResult
	if w.rev != nil {
		res, err = w.client.InspectClauses(body, *w.rev)
	} else {
		res, err = w.client.InspectClauses(body, "best")
	}
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(res))
	}
	if res[0].Reverted {
		message := "contract call reverted"
		if res[0].Data != "" {
			decoded, err := hexutil.Decode(res[0].Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode revert data: %w", err)
			}
			revertReason, err := UnpackRevert(decoded)
			if err == nil {
				message = fmt.Sprintf("contract call reverted: %s", revertReason)
			}
		}
		return nil, errors.New(message)
	}

	if res[0].VMError != "" {
		return nil, fmt.Errorf("VM error: %s", res[0].VMError)
	}

	return res[0], nil
}

// CallInto calls a method and unpacks the result into the provided results interface.
func (w *Caller) CallInto(methodName string, results any, args ...any) error {
	method, ok := w.abi.Methods[methodName]
	if !ok {
		return errors.New("method not found: " + methodName)
	}
	res, err := w.Call(methodName, args...)
	if err != nil {
		return err
	}
	bytes, err := hexutil.Decode(res.Data)
	if err != nil {
		return err
	}

	return method.Outputs.Unpack(results, bytes)
}

func (w *Caller) Clause(methodName string, args ...any) (*tx.Clause, error) {
	return w.ClauseWithVET(big.NewInt(0), methodName, args...)
}

func (w *Caller) ClauseWithVET(vet *big.Int, methodName string, args ...any) (*tx.Clause, error) {
	method, ok := w.abi.Methods[methodName]
	if !ok {
		return nil, errors.New("method not found: " + methodName)
	}
	data, err := method.Inputs.Pack(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack method (%s): %w", methodName, err)
	}
	data = append(method.Id()[:], data...)
	clause := tx.NewClause(&w.addr).WithData(data)
	clause = clause.WithValue(vet)

	return clause, nil
}

// FilterEvents filters events by event name, range, options, and order.
func (w *Caller) FilterEvents(eventName string, eventsRange *events.Range, opts *events.Options, order logdb.Order) ([]events.FilteredEvent, error) {
	event, ok := w.abi.Events[eventName]
	if !ok {
		return nil, errors.New("event not found: " + eventName)
	}
	id := thor.Bytes32(event.Id())
	req := &events.EventFilter{
		Range:   eventsRange,
		Options: opts,
		Order:   order,
		CriteriaSet: []*events.EventCriteria{
			{
				Address: &w.addr,
				TopicSet: events.TopicSet{
					Topic0: &id,
				},
			},
		},
	}

	return w.client.FilterEvents(req)
}
