// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
)

func TestHealth_NewBestBlock(t *testing.T) {
	h := New(time.Duration(thor.BlockInterval) * time.Second)
	blockID := thor.Bytes32{0x01, 0x02, 0x03}

	h.NewBestBlock(blockID)

	if h.bestBlockID == nil || *h.bestBlockID != blockID {
		t.Errorf("expected bestBlockID to be %v, got %v", blockID, h.bestBlockID)
	}

	if time.Since(h.newBestBlock) > time.Second {
		t.Errorf("newBestBlock timestamp is not recent")
	}

	h.BootstrapStatus(true)

	status, err := h.Status()
	require.NoError(t, err)

	assert.True(t, status.Healthy)
}

func TestHealth_ChainSyncStatus(t *testing.T) {
	h := &Health{}

	h.BootstrapStatus(true)
	if !h.bootstrapStatus {
		t.Errorf("expected bootstrapStatus to be true, got false")
	}

	h.BootstrapStatus(false)
	if h.bootstrapStatus {
		t.Errorf("expected bootstrapStatus to be false, got true")
	}

	status, err := h.Status()
	require.NoError(t, err)

	assert.False(t, status.Healthy)
}

func TestHealth_Status(t *testing.T) {
	h := New(time.Second)
	blockID := thor.Bytes32{0x01, 0x02, 0x03}

	h.NewBestBlock(blockID)
	h.BootstrapStatus(true)

	status, err := h.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.Healthy {
		t.Errorf("expected healthy to be true, got false")
	}

	if status.BlockIngestion.ID == nil || *status.BlockIngestion.ID != blockID {
		t.Errorf("expected bestBlock to be %v, got %v", blockID, status.BlockIngestion.ID)
	}

	if status.BlockIngestion.Timestamp == nil || time.Since(*status.BlockIngestion.Timestamp) > time.Second {
		t.Errorf("bestBlockIngestionTimestamp is not recent")
	}

	if !status.ChainBootstrapped {
		t.Errorf("expected chainSync to be true, got false")
	}
}

func TestHealth_Solo(t *testing.T) {
	h := NewSolo(time.Duration(thor.BlockInterval) * time.Second)
	h.NewBestBlock(thor.Bytes32{0x01, 0x02, 0x03})

	status, err := h.Status()
	require.NoError(t, err)

	require.Equal(t, status.ChainBootstrapped, true)
	require.Equal(t, status.Healthy, true)
}
