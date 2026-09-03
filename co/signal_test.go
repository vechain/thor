// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package co

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignalBroadcastBefore(t *testing.T) {
	var sig Signal
	sig.Broadcast()

	var ws []Waiter
	for range 10 {
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
	var sig Signal

	var ws []Waiter
	for range 10 {
		ws = append(ws, sig.NewWaiter())
	}

	sig.Broadcast()

	for _, w := range ws {
		<-w.C()
	}
}
