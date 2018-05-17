// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi

import (
	"encoding/json"
	"errors"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/thor"
)

// ABI holds information about methods and events of contract.
type ABI struct {
	constructor  *Method
	methods      []*Method
	events       []*Event
	nameToMethod map[string]*Method
	nameToEvent  map[string]*Event
	idToMethod   map[MethodID]*Method
	idToEvent    map[thor.Bytes32]*Event
}

// New create an ABI instance.
func New(data []byte) (*ABI, error) {
	var fields []struct {
		Type      string
		Name      string
		Constant  bool
		Anonymous bool
		Inputs    []ethabi.Argument
		Outputs   []ethabi.Argument
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	abi := &ABI{
		nameToMethod: make(map[string]*Method),
		nameToEvent:  make(map[string]*Event),
		idToMethod:   make(map[MethodID]*Method),
		idToEvent:    make(map[thor.Bytes32]*Event),
	}

	for _, field := range fields {
		switch field.Type {
		case "constructor":
			abi.constructor = &Method{
				MethodID{},
				&ethabi.Method{Inputs: field.Inputs},
			}
		// empty defaults to function according to the abi spec
		case "function", "":
			ethMethod := ethabi.Method{
				Name:    field.Name,
				Const:   field.Constant,
				Inputs:  field.Inputs,
				Outputs: field.Outputs,
			}
			var id MethodID
			copy(id[:], ethMethod.Id())
			method := &Method{id, &ethMethod}
			abi.methods = append(abi.methods, method)
			abi.idToMethod[id] = method
			abi.nameToMethod[ethMethod.Name] = method
		case "event":
			ethEvent := ethabi.Event{
				Name:      field.Name,
				Anonymous: field.Anonymous,
				Inputs:    field.Inputs,
			}
			event := newEvent(&ethEvent)
			abi.events = append(abi.events, event)
			abi.idToEvent[event.ID()] = event
			abi.nameToEvent[ethEvent.Name] = event
		}
	}
	return abi, nil
}

// Constructor returns the constructor method if any.
func (a *ABI) Constructor() *Method {
	return a.constructor
}

// Methods returns all methods excluding constructor.
func (a *ABI) Methods() []*Method {
	return a.methods
}

// Events returns all events
func (a *ABI) Events() []*Event {
	return a.events
}

// MethodByInput find the method for given input.
// If the input shorter than MethodID, or method not found, an error returned.
func (a *ABI) MethodByInput(input []byte) (*Method, error) {
	id, err := ExtractMethodID(input)
	if err != nil {
		return nil, err
	}
	m, found := a.idToMethod[id]
	if !found {
		return nil, errors.New("method not found")
	}
	return m, nil
}

// MethodByName find method for the given method name.
func (a *ABI) MethodByName(name string) (*Method, bool) {
	m, found := a.nameToMethod[name]
	return m, found
}

// MethodByID returns method for given method id.
func (a *ABI) MethodByID(id MethodID) (*Method, bool) {
	m, found := a.idToMethod[id]
	return m, found
}

// EventByName find event for the given event name.
func (a *ABI) EventByName(name string) (*Event, bool) {
	e, found := a.nameToEvent[name]
	return e, found
}

// EventByID returns the event for the given event id.
func (a *ABI) EventByID(id thor.Bytes32) (*Event, bool) {
	e, found := a.idToEvent[id]
	return e, found
}
