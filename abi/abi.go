// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi

import (
	"bytes"
	"errors"

	ethabi "github.com/vechain/thor/v2/abi/ethabi"

	"github.com/vechain/thor/v2/thor"
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
	ethABI, err := ethabi.JSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	abi := &ABI{
		nameToMethod: make(map[string]*Method),
		nameToEvent:  make(map[string]*Event),
		idToMethod:   make(map[MethodID]*Method),
		idToEvent:    make(map[thor.Bytes32]*Event),
	}

	abi.constructor = &Method{
		MethodID{},
		&ethABI.Constructor,
	}

	for _, method := range ethABI.Methods {
		var id MethodID
		copy(id[:], method.ID)
		m := &Method{id, &method}
		abi.methods = append(abi.methods, m)
		abi.idToMethod[id] = m
		abi.nameToMethod[method.Name] = m
	}

	for _, event := range ethABI.Events {
		e := newEvent(&event)
		abi.events = append(abi.events, e)
		abi.idToEvent[thor.Bytes32(event.ID)] = e
		abi.nameToEvent[event.Name] = e
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

func UnpackIntoInterface(args *ethabi.Arguments, data []byte, v any) error {
	values, err := args.UnpackValues(data)
	if err != nil {
		return err
	}
	if err := args.Copy(v, values); err != nil {
		return err
	}
	return nil
}
