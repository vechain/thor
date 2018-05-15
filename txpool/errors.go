package txpool

import "errors"

type errBadTx error

func badTx(err string) errBadTx {
	return errBadTx(errors.New(err))
}

type errRejectedTx error

func rejectedTx(err string) errRejectedTx {
	return errRejectedTx(errors.New(err))
}

func IsBadTx(err error) bool {
	_, ok := err.(errBadTx)
	return ok
}

func IsRejectedTx(err error) bool {
	_, ok := err.(errRejectedTx)
	return ok
}
