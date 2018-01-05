package consensus

import "errors"

var (
	errTxsRoot        = errors.New("")
	errGasUsed        = errors.New("")
	errStateRoot      = errors.New("")
	errReceiptsRoot   = errors.New("")
	errParentNotFound = errors.New("")
	errTimestamp      = errors.New("")
	errNumber         = errors.New("")
	errVerify         = errors.New("")
	errKnownBlock     = errors.New("block already known")
)

// 1. 丢弃
// 2. 未来
// 3. 未知
