// Copyright (c) 2018 The VeChainThor developers

package xenv

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRevertError(t *testing.T) {
	assert.False(t, isReverted(nil))
	assert.False(t, isReverted(errors.New("reverted")))
	assert.False(t, isReverted(errors.New("")))

	assert.True(t, isReverted(&errReverted{}))
	assert.True(t, isReverted(&errReverted{message: "reverted"}))
}
