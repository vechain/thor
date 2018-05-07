package txpool

import "github.com/pkg/errors"

var (
	errKnownTx             = errors.New("known transaction")
	errTooLarge            = errors.New("tx too large")
	errExpired             = errors.New("tx expired")
	errIntrisicGasExceeded = errors.New("intrinsic gas exceeds provided gas")
	errInsufficientEnergy  = errors.New("insufficient energy")
	errNegativeValue       = errors.New("negative clause value")
)

func IsErrKnownTx(err error) bool {
	return err == errKnownTx
}

func IsErrTooLarge(err error) bool {
	return err == errTooLarge
}

func IsErrExpired(err error) bool {
	return err == errExpired
}

func IsErrIntrisicGasExceeded(err error) bool {
	return err == errIntrisicGasExceeded
}

func IsErrInsufficientEnergy(err error) bool {
	return err == errInsufficientEnergy
}

func IsErrNegativeValue(err error) bool {
	return err == errNegativeValue
}
