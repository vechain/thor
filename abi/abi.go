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
	nameToMethod map[string]*Method
	nameToEvent  map[string]*Event
	methods      map[MethodID]*Method
	events       map[thor.Bytes32]*Event
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
		methods:      make(map[MethodID]*Method),
		events:       make(map[thor.Bytes32]*Event),
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
			abi.methods[id] = method
			abi.nameToMethod[ethMethod.Name] = method
		case "event":
			ethEvent := ethabi.Event{
				Name:      field.Name,
				Anonymous: field.Anonymous,
				Inputs:    field.Inputs,
			}
			id := thor.Bytes32(ethEvent.Id())
			event := &Event{id, &ethEvent}
			abi.events[id] = event
			abi.nameToEvent[ethEvent.Name] = event
		}
	}
	return abi, nil
}

// Constructor returns the constructor method if any.
func (a *ABI) Constructor() *Method {
	return a.constructor
}

// MethodByInput find the method for given input.
// If the input shorter than MethodID, or method not found, an error returned.
func (a *ABI) MethodByInput(input []byte) (*Method, error) {
	id, err := ExtractMethodID(input)
	if err != nil {
		return nil, err
	}
	m, found := a.methods[id]
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
	m, found := a.methods[id]
	return m, found
}

// EventByName find event for the given event name.
func (a *ABI) EventByName(name string) (*Event, bool) {
	e, found := a.nameToEvent[name]
	return e, found
}

// EventByID returns the event for the given event id.
func (a *ABI) EventByID(id thor.Bytes32) (*Event, bool) {
	e, found := a.events[id]
	return e, found
}
