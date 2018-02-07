package rabi

import (
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// ReversedABI provides methods for unpacking inputs and packing outputs.
type ReversedABI struct {
	rabi           *abi.ABI
	idToMethodName map[uint32]string
}

// New create a new ReversedABI instance.
func New(a *abi.ABI) *ReversedABI {
	rabi := &abi.ABI{
		Constructor: a.Constructor,
		Methods:     make(map[string]abi.Method),
		Events:      make(map[string]abi.Event),
	}
	idToMethodName := make(map[uint32]string, len(a.Methods))
	for n, m := range a.Methods {
		id := binary.BigEndian.Uint32(m.Id())
		idToMethodName[id] = m.Name
		// swap inputs and outputs
		m.Inputs, m.Outputs = m.Outputs, m.Inputs
		rabi.Methods[n] = m
	}
	return &ReversedABI{rabi, idToMethodName}
}

// NameOf find name of input by its first 4 bytes.
func (r *ReversedABI) NameOf(input []byte) (string, error) {
	if len(input) < 4 {
		return "", errors.New("invalid input, len < 4")
	}
	id := binary.BigEndian.Uint32(input)
	return r.idToMethodName[id], nil
}

// UnpackInput unpack input data.
func (r *ReversedABI) UnpackInput(v interface{}, name string, input []byte) error {
	return r.rabi.Unpack(v, name, input[4:])
}

// PackOutput pack output data.
func (r *ReversedABI) PackOutput(name string, args ...interface{}) ([]byte, error) {
	out, err := r.rabi.Pack(name, args...)
	if err != nil {
		return nil, err
	}
	return out[4:], nil
}
