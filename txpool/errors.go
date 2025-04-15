// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

type (
	badTxError      struct{ msg string }
	txRejectedError struct{ msg string }
)

func (e badTxError) Error() string {
	return "bad tx: " + e.msg
}

func (e txRejectedError) Error() string {
	return "tx rejected: " + e.msg
}

// IsBadTx returns whether the given error indicates that tx is bad.
func IsBadTx(err error) bool {
	_, ok := err.(badTxError)
	return ok
}

// IsTxRejected returns whether the given error indicates tx is rejected.
func IsTxRejected(err error) bool {
	_, ok := err.(txRejectedError)
	return ok
}
