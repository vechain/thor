package abi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
)

// ABI holds information about methods and events of contract.
type ABI struct {
	abi            *ethabi.ABI
	idToMethodName map[uint32]string // for fast name finding
}

// New create an ABI instance.
func New(reader io.Reader) (*ABI, error) {
	abi, err := ethabi.JSON(reader)
	if err != nil {
		return nil, err
	}
	removeUnderscore(abi.Constructor.Inputs)
	removeUnderscore(abi.Constructor.Outputs)
	for _, m := range abi.Methods {
		removeUnderscore(m.Inputs)
		removeUnderscore(m.Outputs)
	}
	for _, e := range abi.Events {
		removeUnderscore(e.Inputs)
	}

	idToMethodName := make(map[uint32]string, len(abi.Methods))
	for n, m := range abi.Methods {
		id := binary.BigEndian.Uint32(m.Id())
		idToMethodName[id] = n
	}
	return &ABI{
		&abi,
		idToMethodName,
	}, nil
}

// MethodName find the name of method for given input.
func (a *ABI) MethodName(input []byte) (string, error) {
	if len(input) < 4 {
		return "", errors.New("input too short")
	}
	id := binary.BigEndian.Uint32(input)
	return a.idToMethodName[id], nil
}

// ForMethod create Packer instance for the given method name.
// error returned if method not found.
func (a *ABI) ForMethod(name string) (*MethodPacker, error) {
	if name == "" {
		return &MethodPacker{
			forward: &ethabi.ABI{Constructor: a.abi.Constructor},
		}, nil
	}

	method, ok := a.abi.Methods[name]
	if !ok {
		return nil, errors.New("no such method")
	}
	// method with inputs outputs swapped
	rmethod := method
	rmethod.Inputs, rmethod.Outputs = rmethod.Outputs, rmethod.Inputs

	return &MethodPacker{
		method.Id(),
		name,
		&ethabi.ABI{Methods: map[string]ethabi.Method{name: method}},
		&ethabi.ABI{Methods: map[string]ethabi.Method{name: rmethod}},
	}, nil
}

// ForEvent create event unpacker for the given event name.
// error returned if event not found.
func (a *ABI) ForEvent(name string) (unpack func(output []byte, v interface{}) error, err error) {
	event, ok := a.abi.Events[name]
	if !ok {
		return nil, errors.New("no such event")
	}

	abi := &ethabi.ABI{
		Methods: map[string]ethabi.Method{},
		Events:  map[string]ethabi.Event{name: event},
	}
	return func(output []byte, v interface{}) error {
		return abi.Unpack(v, event.Name, output)
	}, nil
}

// MethodPacker to pack/unpack input/output.
type MethodPacker struct {
	id       []byte
	name     string
	forward  *ethabi.ABI
	reversed *ethabi.ABI
}

// PackInput packs input args into input data.
func (p *MethodPacker) PackInput(args ...interface{}) ([]byte, error) {
	return p.forward.Pack(p.name, args...)
}

// UnpackOutput unpacks output data.
func (p *MethodPacker) UnpackOutput(output []byte, v interface{}) error {
	return p.forward.Unpack(v, p.name, output)
}

// PackOutput packs outputs into output data.
func (p *MethodPacker) PackOutput(args ...interface{}) ([]byte, error) {
	if p.reversed == nil {
		return nil, errors.New("pack ouput unsupported")
	}
	out, err := p.reversed.Pack(p.name, args...)
	if err != nil {
		return nil, err
	}
	return out[4:], nil
}

// UnpackInput unpacks input data into args.
func (p *MethodPacker) UnpackInput(input []byte, v interface{}) error {
	if p.reversed == nil {
		return errors.New("unpack input unsupported")
	}
	if !bytes.HasPrefix(input, p.id) {
		return errors.New("input mismatch")
	}
	return p.reversed.Unpack(v, p.name, input[4:])
}

func removeUnderscore(args []ethabi.Argument) {
	for i := range args {
		arg := &args[i]
		for strings.HasPrefix(arg.Name, "_") {
			arg.Name = arg.Name[1:]
		}
	}
}
