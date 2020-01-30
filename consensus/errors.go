// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"errors"
	"fmt"
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

const (
	strErrTimestampVsParent  = "invalid timestamp: parent = %v, curr = %v"
	strErrTimestampVsNow     = "invalid timestamp: timestamp = %v, now = %v"
	strErrParentID           = "invalid parent block ID"
	strErrGasLimit           = "invalid gas limit: parent = %v, curr = %v"
	strErrGasExceed          = "gas used exceeds limit: limit %v, used %v"
	strErrTotalScoreVsParent = "invalid total score: parent = %v, curr = %v"
	strErrCompSigner         = "signer unavailable: %v"
	strErrSigner             = "invalid signer: signer = %v, err = %v"
	strErrTimestampUnsched   = "timestamp unscheduled: timestamp = %v, signer = %v"
	strErrTotalScore         = "invalid total score: expected =  %v, curr = %v"
	strErrTxsRoot            = "block txs root mismatch: expected = %v, curr = %v"
	strErrBlockedTx          = "tx origin blocked got packed: %v"
	strErrChainTag           = "tx chain tag mismatch: expected = %v, curr = %v"
	strErrFutureTx           = "tx ref future block: ref %v, current %v"
	strErrExpiredTx          = "tx expired: ref %v, current %v, expiration %v"
	strErrStateRoot          = "block state root mismatch: expected = %v, curr = %v"
	strErrReceiptsRoot       = "block receipts root mismatch: expected = %v, curr = %v"
	strErrGasUsed            = "block gas used mismatch: expected = %v, curr = %v"

	strErrZeroRound    = "zero round number"
	strErrZeroEpoch    = "zero epoch number"
	strErrNotCommittee = "not a committee member"
	strErrProof        = "invalid vrf proof"
	strErrNotCandidate = "not a candidate: %v"
)

type consensusType uint8

const (
	ctBlock consensusType = iota
	ctBlockBody
	ctHeader
	ctBlockSummary
	ctEndorsement
	ctTxSet
	ctProposer
	ctLeader
	ctNil
)

// newConsensusError ...
func newConsensusError(t consensusType, strErr string, args ...interface{}) consensusError {
	switch t {
	case ctBlock:
		strErr += "block: "
	case ctBlockSummary:
		strErr += "block summary: "
	case ctHeader:
		strErr += "block header: "
	case ctEndorsement:
		strErr += "endorsement: "
	case ctTxSet:
		strErr += "tx set: "
	case ctProposer:
		strErr += "proposer: "
	case ctLeader:
		strErr += "leader: "
	default:
		// panic("invalid consensus type")
	}
	return consensusError(fmt.Sprintf(strErr, args...))
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
