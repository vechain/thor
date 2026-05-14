// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethconvert

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/vechain/thor/v2/rpc"
)

func TestEthLogsBloom_empty(t *testing.T) {
	bloom := ethLogsBloom(nil)
	require.Len(t, bloom, 256)
	assert.Equal(t, make([]byte, 256), []byte(bloom))

	bloom2 := ethLogsBloom([]*rpc.EthLog{})
	assert.Equal(t, make([]byte, 256), []byte(bloom2))
}

// TestEthLogsBloom_crossCheck verifies our bloom9 implementation matches
// go-ethereum's types.LogsBloom for the same log entries.
func TestEthLogsBloom_crossCheck(t *testing.T) {
	// ERC-20 Transfer(address indexed from, address indexed to, uint256 value)
	transferTopic := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	contractAddr := common.HexToAddress("0x0000000000000000000000000000000000000abc")
	fromAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	toAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fromTopic := common.BytesToHash(fromAddr.Bytes())
	toTopic := common.BytesToHash(toAddr.Bytes())

	ethLog := &rpc.EthLog{
		Address: contractAddr,
		Topics:  []common.Hash{transferTopic, fromTopic, toTopic},
		Data:    []byte{},
	}

	// Reference: go-ethereum types.LogsBloom
	gethLog := &ethtypes.Log{
		Address: contractAddr,
		Topics:  []common.Hash{transferTopic, fromTopic, toTopic},
		Data:    []byte{},
	}
	gethBin := ethtypes.LogsBloom([]*ethtypes.Log{gethLog})
	expected := make([]byte, 256)
	b := gethBin.Bytes()
	copy(expected[256-len(b):], b)

	got := ethLogsBloom([]*rpc.EthLog{ethLog})
	assert.Equal(t, expected, []byte(got), "bloom must match go-ethereum reference")

	// Verify the bloom contains the expected entries via BloomLookup.
	var bloom256 [256]byte
	copy(bloom256[:], got)
	ethBloom := ethtypes.BytesToBloom(bloom256[:])
	assert.True(t, ethtypes.BloomLookup(ethBloom, contractAddr), "bloom should contain contract address")
	assert.True(t, ethtypes.BloomLookup(ethBloom, transferTopic), "bloom should contain Transfer topic")
}

func TestEthTransactionsRoot_empty(t *testing.T) {
	root := ethTransactionsRoot(nil)
	assert.Equal(t, ethtypes.EmptyRootHash, root, "empty tx list must produce Ethereum empty trie root")
}

func TestEthReceiptsRoot_empty(t *testing.T) {
	root := ethReceiptsRoot(nil)
	assert.Equal(t, ethtypes.EmptyRootHash, root, "empty receipt list must produce Ethereum empty trie root")
}

func TestEthReceiptWireBytes(t *testing.T) {
	bloom := make([]byte, 256)
	bloom[255] = 0x01 // one non-zero bit

	rec := &rpc.EthReceipt{
		Status:            1,
		CumulativeGasUsed: 21000,
		LogsBloom:         bloom,
		Logs:              []*rpc.EthLog{},
	}

	b := ethReceiptWireBytes(rec)
	require.Greater(t, len(b), 1)
	assert.Equal(t, byte(0x02), b[0], "first byte must be the EIP-1559 receipt type 0x02")

	// Status 0 (reverted) must produce a different encoding.
	rec.Status = 0
	bReverted := ethReceiptWireBytes(rec)
	assert.Equal(t, byte(0x02), bReverted[0])
	assert.NotEqual(t, b, bReverted, "success and reverted receipts must encode differently")
}
