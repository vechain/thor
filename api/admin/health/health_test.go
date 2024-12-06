// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealth_isNetworkProgressing(t *testing.T) {
	h := &Health{}

	now := time.Now()

	tests := []struct {
		name                string
		bestBlockTimestamp  time.Time
		expectedProgressing bool
	}{
		{
			name:                "Progressing - block within timeBetweenBlocks",
			bestBlockTimestamp:  now.Add(-5 * time.Second),
			expectedProgressing: true,
		},
		{
			name:                "Not Progressing - block outside timeBetweenBlocks",
			bestBlockTimestamp:  now.Add(-25 * time.Second),
			expectedProgressing: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isProgressing := h.isNetworkProgressing(now, tt.bestBlockTimestamp, defaultBlockTolerance)
			assert.Equal(t, tt.expectedProgressing, isProgressing, "isNetworkProgressing result mismatch")
		})
	}
}

func TestHealth_isNodeConnectedP2P(t *testing.T) {
	h := &Health{}

	tests := []struct {
		name              string
		peerCount         int
		expectedConnected bool
	}{
		{
			name:              "Connected - more than one peer",
			peerCount:         3,
			expectedConnected: true,
		},
		{
			name:              "Not Connected - one or fewer peers",
			peerCount:         1,
			expectedConnected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isConnected := h.isNodeConnectedP2P(tt.peerCount, defaultMinPeerCount)
			assert.Equal(t, tt.expectedConnected, isConnected, "isNodeConnectedP2P result mismatch")
		})
	}
}
