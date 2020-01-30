// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"errors"
)

var (
	errFutureBlock   = errors.New("block in the future")
	errParentMissing = errors.New("parent block is missing")
	errKnownBlock    = errors.New("block already in the chain")

	// errRound       = errors.New("Invalid round number")
	// errParent      = errors.New("Invalid parent")
	// errSig         = errors.New("Invalid signature")
	// errTimestamp   = errors.New("Invalid timestamp")
	// errFutureEpoch = errors.New("Future epoch number")
	// errZeroRound   = errors.New("Zero round number")
	// errZeroEpoch   = errors.New("Zero epoch number")
	// errZeroChain   = errors.New("Zero chain length")

	// errVrfProof     = errors.New("VRF proof verfication failed")
	// errNotCommittee = errors.New("Not a committee member")
)

type consensusError string

func (err consensusError) Error() string {
	return string(err)
}

// IsFutureBlock returns if the error indicates that the block should be
// processed later.
func IsFutureBlock(err error) bool {
	return err == errFutureBlock
}

// IsParentMissing ...
func IsParentMissing(err error) bool {
	return err == errParentMissing
}

// IsKnownBlock returns if the error means the block was already in the chain.
func IsKnownBlock(err error) bool {
	return err == errKnownBlock
}

// IsCritical returns if the error is consensus related.
func IsCritical(err error) bool {
	_, ok := err.(consensusError)
	return ok
}
