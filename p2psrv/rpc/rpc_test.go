// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/p2p"
)

func TestHandleResult_RejectsOversizedResultBeforeDecode(t *testing.T) {
	r := &RPC{
		doneCh:   make(chan struct{}),
		pendings: make(map[uint32]*resultListener),
		logger:   logger,
	}

	decoded := false
	r.pendings[1] = &resultListener{
		msgCode:       0x07,
		maxResultSize: 1024,
		onResult: func(msg *p2p.Msg) error {
			decoded = true
			return nil
		},
	}

	msg := &p2p.Msg{Code: 0x07, Size: 2048}
	err := r.handleResult(1, msg)

	assert.ErrorIs(t, err, errResultTooLarge)
	assert.False(t, decoded, "onResult ran; the allocation already happened")
}

func TestHandleResult_AcceptsResultWithinLimit(t *testing.T) {
	r := &RPC{
		doneCh:   make(chan struct{}),
		pendings: make(map[uint32]*resultListener),
		logger:   logger,
	}

	decoded := false
	r.pendings[1] = &resultListener{
		msgCode:       0x07,
		maxResultSize: 1024,
		onResult: func(msg *p2p.Msg) error {
			decoded = true
			return nil
		},
	}

	msg := &p2p.Msg{Code: 0x07, Size: 512}
	require.NoError(t, r.handleResult(1, msg))
	assert.True(t, decoded)
}

func TestHandleResult_ZeroMaxResultSizeAllowsAnySize(t *testing.T) {
	r := &RPC{
		doneCh:   make(chan struct{}),
		pendings: make(map[uint32]*resultListener),
		logger:   logger,
	}

	decoded := false
	r.pendings[1] = &resultListener{
		msgCode:       0x07,
		maxResultSize: 0,
		onResult: func(msg *p2p.Msg) error {
			decoded = true
			return nil
		},
	}

	msg := &p2p.Msg{Code: 0x07, Size: 100 * 1024 * 1024}
	require.NoError(t, r.handleResult(1, msg))
	assert.True(t, decoded)
}
