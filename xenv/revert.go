package xenv

import (
	"errors"

	"github.com/vechain/thor/v2/abi"
)

var revertABI = []byte(`[{"name": "Error","type": "function","inputs": [{"name": "message","type": "string"}]}]`)

type errReverted struct {
	message string
}

func (e *errReverted) Error() string {
	return e.message
}

func (e *errReverted) Bytes() []byte {
	abi, _ := abi.New(revertABI)
	method, _ := abi.MethodByName("Error")
	data, _ := method.EncodeInput(e.message)
	return data
}

func isReverted(err any) bool {
	e, ok := err.(error)
	if !ok {
		return false
	}
	var target *errReverted
	return errors.As(e, &target)
}
