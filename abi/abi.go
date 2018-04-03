package abi

import (
	"encoding/json"

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
// If the input shorter than MethodID, an error returned.
// Note that the returned Method may be nil even no error.
func (a *ABI) MethodByInput(input []byte) (*Method, error) {
	id, err := ExtractMethodID(input)
	if err != nil {
		return nil, err
	}

	return a.MethodByID(id), nil
}

// MethodByName find method for the given method name.
func (a *ABI) MethodByName(name string) *Method {
	return a.nameToMethod[name]
}

// MethodByID returns method for given method id.
func (a *ABI) MethodByID(id MethodID) *Method {
	return a.methods[id]
}

// EventByName find event for the given event name.
func (a *ABI) EventByName(name string) *Event {
	return a.nameToEvent[name]
}

// EventByID returns the event for the given event id.
func (a *ABI) EventByID(id thor.Bytes32) *Event {
	return a.events[id]
}
