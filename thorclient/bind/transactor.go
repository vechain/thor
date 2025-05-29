// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"math/big"
)

// Transactor is a generic contract wrapper to build and send transactions.
// It also allows calling methods and filtering events since it embeds a Caller.
type Transactor struct {
	*Caller
	signer Signer
}

func NewTransactor(signer Signer, caller *Caller) *Transactor {
	return &Transactor{
		Caller: caller,
		signer: signer,
	}
}

func (w *Transactor) Sender(methodName string, args ...any) *Sender {
	return w.SenderWithVET(big.NewInt(0), methodName, args...)
}

func (w *Transactor) SenderWithVET(vet *big.Int, methodName string, args ...any) *Sender {
	return newSender(w, vet, methodName, args...)
}
