// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/co"
)

func TestSignalBroadcastBefore(t *testing.T) {
	var sig co.Signal
	sig.Broadcast()

	var ws []co.Waiter
	for i := 0; i < 10; i++ {
		ws = append(ws, sig.NewWaiter())
	}

	var n int
	for _, w := range ws {
		select {
		case <-w.C():
		default:
			n++
		}
	}
	assert.Equal(t, 10, n)
}

func TestSignalBroadcastAfterWait(t *testing.T) {
	var sig co.Signal

	var ws []co.Waiter
	for i := 0; i < 10; i++ {
		ws = append(ws, sig.NewWaiter())
	}

	sig.Broadcast()

	for _, w := range ws {
		<-w.C()
	}
}
