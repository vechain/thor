// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// encodedBlock returns the wire encoding of a block whose Header().Number() == num.
func encodedBlock(t *testing.T, num uint32) rlp.RawValue {
	t.Helper()
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:4], num-1)
	data, err := rlp.EncodeToBytes(new(block.Builder).ParentID(parentID).Build())
	require.NoError(t, err)
	return data
}

func runDecodeAndWarmup(t *testing.T, batch rawBlockBatch) ([]*block.Block, error) {
	t.Helper()
	rawBatches := make(chan rawBlockBatch, 1)
	rawBatches <- batch
	close(rawBatches)
	warmedUp := make(chan *block.Block, 2048)

	err := decodeAndWarmupBatches(context.Background(), rawBatches, warmedUp)
	close(warmedUp)
	var got []*block.Block
	for blk := range warmedUp {
		got = append(got, blk)
	}
	return got, err
}

func TestDecodeAndWarmupBatches(t *testing.T) {
	t.Run("decodes a well-formed batch in sequence", func(t *testing.T) {
		got, err := runDecodeAndWarmup(t, rawBlockBatch{
			rawBlocks: []rlp.RawValue{encodedBlock(t, 10), encodedBlock(t, 11)},
			startNum:  10,
		})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, uint32(10), got[0].Header().Number())
		assert.Equal(t, uint32(11), got[1].Header().Number())
	})

	t.Run("rejects a block whose number breaks the sequence", func(t *testing.T) {
		got, err := runDecodeAndWarmup(t, rawBlockBatch{
			rawBlocks: []rlp.RawValue{encodedBlock(t, 10), encodedBlock(t, 99)},
			startNum:  10,
		})
		assert.EqualError(t, err, "broken sequence")
		assert.Len(t, got, 1)
	})

	t.Run("rejects an undecodable block", func(t *testing.T) {
		got, err := runDecodeAndWarmup(t, rawBlockBatch{
			rawBlocks: []rlp.RawValue{{0x83, 0x01, 0x02, 0x03}},
			startNum:  10,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid block")
		assert.Empty(t, got)
	})
}

func TestDecodeAndWarmupBatches_SizeFromWire(t *testing.T) {
	raw := encodedBlock(t, 10)
	got, err := runDecodeAndWarmup(t, rawBlockBatch{rawBlocks: []rlp.RawValue{raw}, startNum: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, len(raw), got[0].Size())
}
