// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
)

func TestUpdateProcessed(t *testing.T) {
	var s blockStats

	n := 10
	txs := 5
	exec := mclock.AbsTime(100)
	commit := mclock.AbsTime(200)
	realTime := mclock.AbsTime(300)
	usedGas := uint64(5000)

	s.UpdateProcessed(n, txs, exec, commit, realTime, usedGas)

	assert.Equal(t, 10, s.processed, "processed count mismatch")
	assert.Equal(t, 5, s.txs, "txs count mismatch")
	assert.Equal(t, mclock.AbsTime(100), s.exec, "exec time mismatch")
	assert.Equal(t, mclock.AbsTime(200), s.commit, "commit time mismatch")
	assert.Equal(t, mclock.AbsTime(300), s.real, "real time mismatch")
	assert.Equal(t, uint64(5000), s.usedGas, "usedGas mismatch")
}

func TestLogContext(t *testing.T) {
	var s blockStats
	// Initialize blockStats with some data
	s.txs = 10
	s.usedGas = 5000000 // 5 million gas
	s.exec = mclock.AbsTime(2000)
	s.commit = mclock.AbsTime(1000)
	s.real = mclock.AbsTime(3000)

	// Mock block header
	var mockHeader block.Header

	logContext := s.LogContext(&mockHeader)

	// Verify the log context elements
	assert.Equal(t, 10, logContext[1], "txs count mismatch")
	assert.Equal(t, 5.0, logContext[3], "used gas in mgas mismatch")
}
