package txpool

import (
	"fmt"
)

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
	return fmt.Sprintf("bad tx err: %v", e.msg)
}

type rejectedTxErr struct {
	msg string
}

func (e rejectedTxErr) Error() string {
	return fmt.Sprintf("rejected tx err: %v", e.msg)
}
