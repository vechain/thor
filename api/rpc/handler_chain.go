// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
)

func init() {
	register("eth_chainId", handleChainID)
	register("eth_blockNumber", handleBlockNumber)
	register("eth_syncing", handleSyncing)
}

// handleChainID returns CHAIN_ID per chainid.md §5 / §9:
//   - Pre-INTERSTELLAR: the 32-byte genesis ID interpreted as uint256 (the
//     legacy value that Thor's CHAINID opcode has always returned).
//   - Post-INTERSTELLAR: the 2-byte uint16BE(genesisID[30:32]) value via
//     thor.ChainID — the EIP-155-compatible chain id wallets expect.
//
// Always a QUANTITY hex string.
func handleChainID(_ context.Context, s *Server, _ json.RawMessage) (any, *RPCError) {
	best := s.repo.BestBlockSummary()
	genesisID := s.repo.GenesisBlock().Header().ID()

	var v *big.Int
	if thor.IsForked(best.Header.Number(), s.forkConfig.INTERSTELLAR) {
		v = new(big.Int).SetUint64(thor.ChainID(genesisID))
	} else {
		v = new(big.Int).SetBytes(genesisID.Bytes())
	}
	return (*hexutil.Big)(v), nil
}

// handleBlockNumber returns the best block height as a QUANTITY.
func handleBlockNumber(_ context.Context, s *Server, _ json.RawMessage) (any, *RPCError) {
	best := s.repo.BestBlockSummary()
	return hexutil.Uint64(best.Header.Number()), nil
}

// handleSyncing returns false (not syncing). Thor solo / dev nodes are
// always considered synced at the JSON-RPC surface; when a real comm module
// is wired in Phase 5 this handler can inspect Communicator.IsSynced and
// emit the {startingBlock, currentBlock, highestBlock} object per spec §5
// / Deferred §13.13.
func handleSyncing(_ context.Context, s *Server, _ json.RawMessage) (any, *RPCError) {
	return false, nil
}
