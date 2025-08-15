package xenv

import "errors"

var revertABI = []byte(`[{"name": "Error","type": "function","inputs": [{"name": "message","type": "string"}]}]`)

type ErrReverted struct {
	ReturnData []byte
}

func (e ErrReverted) Error() string {
	return "reverted"
}

func isReverted(err any) bool {
	e, ok := err.(error)
	if !ok {
		return false
	}

	return errors.As(e, &ErrReverted{})
}
