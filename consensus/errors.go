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

type consensusError struct {
	trace    string
	strErr   string
	strData  string
	data     []interface{}
	strCause string
}

func (err consensusError) Error() string {
	s := err.trace + err.strErr
	if len(err.strData) > 0 {
		s = fmt.Sprintf(s+": "+err.strData, err.data...)
	}
	if len(err.strCause) > 0 {
		s += fmt.Sprintf("\n\tCaused by: ") + err.strCause
	}
	return s
}

func (err consensusError) ErrorMsg() string {
	return err.strErr
}

func (err consensusError) AddTraceInfo(tr string) consensusError {
	err.trace = tr + err.trace
	return err
}

const (
	strErrTimestamp = "invalid timestamp"
	// strErrTimestampVsNow     = "invalid timestamp: timestamp = %v, now = %v"
	strErrParentID         = "invalid parent block ID"
	strErrGasLimit         = "invalid gas limit"
	strErrGasExceed        = "gas used exceeds limit"
	strErrSignature        = "invalid signature"
	strErrSigner           = "invalid signer"
	strErrTimestampUnsched = "timestamp unscheduled"
	strErrTotalScore       = "invalid total score"
	strErrTxsRoot          = "txs root mismatch"
	strErrBlockedTxOrign   = "tx origin blocked"
	strErrChainTag         = "tx chain tag mismatch"
	strErrFutureTx         = "tx refs future block"
	strErrExpiredTx        = "tx expired"
	strErrStateRoot        = "block state root mismatch"
	strErrReceiptsRoot     = "block receipts root mismatch"
	strErrGasUsed          = "block gas used mismatch"
	strErrTxFeatures       = "invalid tx features"

	strErrZeroRound    = "zero round number"
	strErrZeroEpoch    = "zero epoch number"
	strErrNotCommittee = "not a committee member"
	strErrProof        = "invalid vrf proof"
	strErrNotCandidate = "not a candidate"

	strDataParent    = "parent=%v"
	strDataTimestamp = "timestamp=%v"
	strDataNowTime   = "now=%v"
	strDataAddr      = "signer=%v"
	strDataSingleVal = "%v"
	strDataCurr      = "curr=%v"
	strDataExpected  = "expected=%v"
	strDataRef       = "ref=%v"
	strDataExp       = "exp=%v"
	strDataLocal     = "local=%v"
)

// Trace where the error is generated
const (
	trBlockBody    = "body: "
	trHeader       = "header: "
	trBlockSummary = "summary: "
	trEndorsement  = "endoresement: "
	trTxSet        = "tx set: "
	trProposer     = "proposer: "
	trLeader       = "leader: "
	trNil          = ""
)

// newConsensusError ...
func newConsensusError(tr string, strErr string, strData []string, data []interface{}, strCause string) consensusError {
	var s string
	for _, str := range strData {
		s += str + ", "
	}
	if len(s) > 2 {
		s = s[:len(s)-2]
	}

	return consensusError{
		trace:    tr,
		strErr:   strErr,
		strData:  s,
		data:     data,
		strCause: strCause,
	}
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
