// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer

import "errors"

var (
	errGasLimitReached       = errors.New("gas limit reached")
	errTxNotAdoptableNow     = errors.New("tx not adoptable now")
	errTxNotAdoptableForever = errors.New("tx not adoptable forever")
	errKnownTx               = errors.New("known tx")
)

// IsGasLimitReached block if full of txs.
func IsGasLimitReached(err error) bool {
	return errors.Is(err, errGasLimitReached)
}

// IsTxNotAdoptableNow tx can not be adopted now.
func IsTxNotAdoptableNow(err error) bool {
	return errors.Is(err, errTxNotAdoptableNow)
}

// IsBadTx not a valid tx.
func IsBadTx(err error) bool {
	return errors.As(err, &badTxError{})
}

type badTxError struct {
	msg string
}

func (e badTxError) Error() string {
	return "bad tx: " + e.msg
}
