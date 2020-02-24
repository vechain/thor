// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errFutureBlock   = errors.New("block in the future")
	errParentMissing = errors.New("parent block is missing")
	errKnownBlock    = errors.New("block already in the chain")
)

type consensusError struct {
	trace    []string      // strings that showing the exec path
	strErr   string        // error message
	strData  []string      // formatted strings for printing data
	data     []interface{} // data to be reported
	strCause string        // cause
}

func (err consensusError) Error() string {
	// s := err.trace + err.strErr
	s := strings.Join(err.trace, ": ")
	if len(err.strData) > 0 {
		// s = fmt.Sprintf(s+": "+err.strData, err.data...)
		s = fmt.Sprintf(strings.Join(err.strData, "=%v, ")+"=%v", err.data...)
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
	// err.trace = tr + err.trace
	err.trace = append([]string{tr}, err.trace...)
	return err
}

const (
	strErrTimestamp        = "invalid timestamp"
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

	strDataParent    = "parent"
	strDataTimestamp = "timestamp"
	strDataNowTime   = "now"
	strDataAddr      = "signer"
	strDataSingleVal = ""
	strDataCurr      = "curr"
	strDataExpected  = "expected"
	strDataRef       = "ref"
	strDataExp       = "exp"
	strDataLocal     = "local"
)

// Trace where the error is generated
const (
	trBlockBody    = "body"
	trHeader       = "header"
	trBlockSummary = "summary"
	trEndorsement  = "endoresement"
	trTxSet        = "tx set"
	trProposer     = "proposer"
	trLeader       = "leader"
	trNil          = ""
)

// newConsensusError ...
func newConsensusError(tr string, strErr string, strData []string, data []interface{}, strCause string) consensusError {
	// var s string
	// for _, str := range strData {
	// 	s += str + ", "
	// }
	// if len(s) > 2 {
	// 	s = s[:len(s)-2]
	// }

	return consensusError{
		trace:    []string{tr},
		strErr:   strErr,
		strData:  strData,
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
