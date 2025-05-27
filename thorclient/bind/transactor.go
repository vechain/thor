package bind

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// Transactor is a generic contract wrapper to build and send transactions.
type Transactor struct {
	*Caller
	signer Signer
}

func NewTransactor(client *thorclient.Client, signer Signer, abiData []byte, address thor.Address) (*Transactor, error) {
	caller, err := NewCaller(client, abiData, address)
	if err != nil {
		return nil, fmt.Errorf("failed to create caller: %w", err)
	}
	if signer == nil {
		return nil, errors.New("signer cannot be nil")
	}
	return &Transactor{
		Caller: caller,
		signer: signer,
	}, nil
}

func (w *Transactor) Sender(methodName string, args ...any) *Sender {
	return w.SenderWithVET(big.NewInt(0), methodName, args...)
}

func (w *Transactor) SenderWithVET(vet *big.Int, methodName string, args ...any) *Sender {
	return newSender(w, vet, methodName, args...)
}
