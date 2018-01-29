package consensus

import "errors"

var (
	errTxsRoot      = errors.New("txs root in header !equal in block body")
	errGasUsed      = errors.New("gas used in header !equal execute block")
	errStateRoot    = errors.New("state root in header !equal execute block")
	errReceiptsRoot = errors.New("receipts root in header !equal execute block")
	errGasLimit     = errors.New("current gas limit -- parent")
	errTimestamp    = errors.New("current timestamp -- parent")
	errTransaction  = errors.New("transaction in block body is bad")
	errSchedule     = errors.New("schedule err")
	errTotalScore   = errors.New("total score err")

	errDelay          = errors.New("timestamp > (current time + thor.BlockInterval)")
	errParentNotFound = errors.New("parent not found in current chain")
)

// IsDelayBlock judge whether block is delay.
func IsDelayBlock(err error) bool {
	return err == errParentNotFound || err == errDelay
}
