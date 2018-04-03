package abi

import (
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/thor"
)

// Event see abi.Event in go-ethereum.
type Event struct {
	id    thor.Bytes32
	event *ethabi.Event
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
	return e.event.Inputs.Pack(args...)
}

// Decode decodes event data.
func (e *Event) Decode(data []byte, v interface{}) error {
	return e.event.Inputs.Unpack(v, data)
}
