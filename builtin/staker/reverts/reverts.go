// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package reverts

import (
	"errors"
)

type ErrRevert struct {
	message string
}

func New(message string) *ErrRevert {
	return &ErrRevert{
		message: message,
	}
}

func (e *ErrRevert) Error() string {
	return e.message
}

func IsRevertErr(err any) bool {
	if err == nil {
		return false
	}
	e, ok := err.(error)
	if !ok {
		return false
	}
	var ve *ErrRevert
	return errors.As(e, &ve)
}
