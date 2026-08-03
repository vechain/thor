// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransactionErrorClassification(t *testing.T) {
	bad := badTxError{"malformed"}
	rejected := txRejectedError{"policy"}
	plain := errors.New("storage failure")

	assert.True(t, IsBadTx(bad))
	assert.False(t, IsTxRejected(bad))
	assert.True(t, IsTxRejected(rejected))
	assert.False(t, IsBadTx(rejected))
	assert.False(t, IsBadTx(plain))
	assert.False(t, IsTxRejected(plain))
	assert.False(t, IsBadTx(nil))
	assert.False(t, IsTxRejected(nil))
}
