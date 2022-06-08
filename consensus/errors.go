// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"errors"
)

var errFutureBlock = errors.New("block in the future")

type consensusError string

func (err consensusError) Error() string {
	return string(err)
}

// IsFutureBlock returns if the error indicates that the block should be
// processed later.
func IsFutureBlock(err error) bool {
	return err == errFutureBlock
}

// IsCritical returns if the error is consensus related.
func IsCritical(err error) bool {
	_, ok := err.(consensusError)
	return ok
}
