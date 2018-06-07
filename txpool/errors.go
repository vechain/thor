// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

func IsBadTx(err error) bool {
	_, ok := err.(badTxErr)
	return ok
}

func IsRejectedTx(err error) bool {
	_, ok := err.(rejectedTxErr)
	return ok
}

type badTxErr struct {
	msg string
}

func (e badTxErr) Error() string {
	return e.msg
}

type rejectedTxErr struct {
	msg string
}

func (e rejectedTxErr) Error() string {
	return e.msg
}
