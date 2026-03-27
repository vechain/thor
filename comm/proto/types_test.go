// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

// mockRPC is a mock implementation of the RPC interface for testing
type mockRPC struct {
	callFunc func(ctx context.Context, msgCode uint64, arg any, result any) error
}

func (m *mockRPC) Notify(ctx context.Context, msgCode uint64, arg any) error {
	return nil
}

func (m *mockRPC) Call(ctx context.Context, msgCode uint64, arg any, result any) error {
	if m.callFunc != nil {
		return m.callFunc(ctx, msgCode, arg, result)
	}
	return nil
}

func TestGetBlockByID_SizeLimit(t *testing.T) {
	ctx := context.Background()
	blockID := thor.Bytes32{}

	t.Run("returns error when result exceeds limit", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				// Simulate receiving 2 blocks (exceeds MaxBlockByIDResult = 1)
				res := result.(*[]rlp.RawValue)
				*res = []rlp.RawValue{
					[]byte{0x01},
					[]byte{0x02},
				}
				return nil
			},
		}

		_, err := GetBlockByID(ctx, mock, blockID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("accepts empty result", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				// Simulate empty result
				res := result.(*[]rlp.RawValue)
				*res = []rlp.RawValue{}
				return nil
			},
		}

		block, err := GetBlockByID(ctx, mock, blockID)
		assert.NoError(t, err)
		assert.Nil(t, block)
	})

	t.Run("accepts single block", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				// Simulate single block
				res := result.(*[]rlp.RawValue)
				*res = []rlp.RawValue{[]byte{0x01}}
				return nil
			},
		}

		block, err := GetBlockByID(ctx, mock, blockID)
		assert.NoError(t, err)
		assert.NotNil(t, block)
	})
}

func TestGetBlocksFromNumber_SizeLimit(t *testing.T) {
	ctx := context.Background()
	num := uint32(1)

	t.Run("returns error when result exceeds limit", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				// Simulate receiving 1025 blocks (exceeds MaxBlocksFromNumber = 1024)
				res := result.(*[]rlp.RawValue)
				oversized := make([]rlp.RawValue, MaxBlocksFromNumber+1)
				for i := range oversized {
					oversized[i] = []byte{byte(i)}
				}
				*res = oversized
				return nil
			},
		}

		_, err := GetBlocksFromNumber(ctx, mock, num)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("accepts empty result", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				res := result.(*[]rlp.RawValue)
				*res = []rlp.RawValue{}
				return nil
			},
		}

		blocks, err := GetBlocksFromNumber(ctx, mock, num)
		assert.NoError(t, err)
		assert.Empty(t, blocks)
	})

	t.Run("accepts maximum allowed blocks", func(t *testing.T) {
		mock := &mockRPC{
			callFunc: func(ctx context.Context, msgCode uint64, arg any, result any) error {
				// Simulate exactly MaxBlocksFromNumber blocks
				res := result.(*[]rlp.RawValue)
				blocks := make([]rlp.RawValue, MaxBlocksFromNumber)
				for i := range blocks {
					blocks[i] = []byte{byte(i)}
				}
				*res = blocks
				return nil
			},
		}

		blocks, err := GetBlocksFromNumber(ctx, mock, num)
		assert.NoError(t, err)
		assert.Len(t, blocks, MaxBlocksFromNumber)
	})
}
