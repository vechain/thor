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

// ForMethod create MethodCodec instance for the given method name.
// error returned if method not found.
func (a *ABI) ForMethod(name string) (*MethodCodec, error) {
	if name == "" {
		return &MethodCodec{
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

	return &MethodCodec{
		method.Id(),
		name,
		&ethabi.ABI{Methods: map[string]ethabi.Method{name: method}},
		&ethabi.ABI{Methods: map[string]ethabi.Method{name: rmethod}},
	}, nil
}

// MustForMethod create MethodCodec instance for the given method name.
// panic if not found.
func (a *ABI) MustForMethod(name string) *MethodCodec {
	codec, err := a.ForMethod(name)
	if err != nil {
		panic(err)
	}
	return codec
}

// ForEvent create event decoder for the given event name.
// error returned if event not found.
func (a *ABI) ForEvent(name string) (decode func(output []byte, v interface{}) error, err error) {
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

// MustForEvent create event decoder for the given event name.
// panic if not found.
func (a *ABI) MustForEvent(name string) func(output []byte, v interface{}) error {
	dec, err := a.ForEvent(name)
	if err != nil {
		panic(err)
	}
	return dec
}

// MethodCodec to encode/decode input/output.
type MethodCodec struct {
	id       []byte
	name     string
	forward  *ethabi.ABI
	reversed *ethabi.ABI
}

// Name returns method name.
func (mc *MethodCodec) Name() string {
	return mc.name
}

// EncodeInput encodes input args into input data.
func (mc *MethodCodec) EncodeInput(args ...interface{}) ([]byte, error) {
	return mc.forward.Pack(mc.name, args...)
}

// DecodeOutput decodes output data.
func (mc *MethodCodec) DecodeOutput(output []byte, v interface{}) error {
	return mc.forward.Unpack(v, mc.name, output)
}

// EncodeOutput encodes outputs into output data.
func (mc *MethodCodec) EncodeOutput(args ...interface{}) ([]byte, error) {
	if mc.reversed == nil {
		return nil, errors.New("encode ouput unsupported")
	}
	out, err := mc.reversed.Pack(mc.name, args...)
	if err != nil {
		return nil, err
	}
	return out[4:], nil
}

// DecodeInput decodes input data into args.
func (mc *MethodCodec) DecodeInput(input []byte, v interface{}) error {
	if mc.reversed == nil {
		return errors.New("decode input unsupported")
	}
	if !bytes.HasPrefix(input, mc.id) {
		return errors.New("input mismatch")
	}
	return mc.reversed.Unpack(v, mc.name, input[4:])
}

// decode multi args into struct requires exported field names.
// while solidity's function args usually are underscored.
// so we should remove underscores.
func removeUnderscore(args []ethabi.Argument) {
	for i := range args {
		arg := &args[i]
		for strings.HasPrefix(arg.Name, "_") {
			arg.Name = arg.Name[1:]
		}
	}
}
