package consensus

import "errors"

var (
	errFutureBlock    = errors.New("block in the future")
	errParentNotFound = errors.New("parent block not found")
	errKnownBlock     = errors.New("block already in the chain")
)

// IsFutureBlock returns if the error indicates that the block should be
// processed later.
func IsFutureBlock(err error) bool {
	return err == errFutureBlock
}

func IsParentNotFound(err error) bool {
	return err == errParentNotFound
}

// IsKnownBlock returns if the error means the block was already in the chain.
func IsKnownBlock(err error) bool {
	return err == errKnownBlock
}
