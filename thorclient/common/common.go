package common

import (
	"fmt"

	"github.com/vechain/thor/v2/thor"
)

var (
	NotFoundErr     = fmt.Errorf("not found")
	Not200StatusErr = fmt.Errorf("not 200 status code")
)

type TxSendResult struct {
	ID *thor.Bytes32
}
