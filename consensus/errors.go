package consensus

import (
	"errors"
)

var (
	errFutureBlock   = errors.New("block in the future")
	errParentMissing = errors.New("parent block is missing")
	errKnownBlock    = errors.New("block already in the chain")
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
