// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package abi

import (
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/thor"
)

// Event see abi.Event in go-ethereum.
type Event struct {
	id                 thor.Bytes32
	event              *ethabi.Event
	argsWithoutIndexed ethabi.Arguments
}

func newEvent(event *ethabi.Event) *Event {
	var argsWithoutIndexed ethabi.Arguments
	for _, arg := range event.Inputs {
		if !arg.Indexed {
			argsWithoutIndexed = append(argsWithoutIndexed, arg)
		}
	}
	return &Event{
		thor.Bytes32(event.Id()),
		event,
		argsWithoutIndexed,
	}
}

// ID returns event id.
func (e *Event) ID() thor.Bytes32 {
	return e.id
}

// Name returns event name.
func (e *Event) Name() string {
	return e.event.Name
}

// Encode encodes args to data.
func (e *Event) Encode(args ...interface{}) ([]byte, error) {
	return e.argsWithoutIndexed.Pack(args...)
}

// Decode decodes event data.
func (e *Event) Decode(data []byte, v interface{}) error {
	return e.argsWithoutIndexed.Unpack(v, data)
}
