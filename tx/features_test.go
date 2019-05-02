package tx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/tx"
)

func TestFeatures(t *testing.T) {
	var f tx.Features

	assert.Zero(t, f)
	assert.False(t, f.IsDelegated())

	f.SetDelegated(true)
	assert.True(t, f.IsDelegated())

	f.SetDelegated(false)
	assert.False(t, f.IsDelegated())

	f.SetDelegated(false)
	assert.False(t, f.IsDelegated())
}
