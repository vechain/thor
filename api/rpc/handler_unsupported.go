// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

func init() {
	register("eth_createAccessList", handleCreateAccessList)
	register("eth_getProof", handleGetProof)
	register("eth_getUncleCountByBlockHash", handleZeroUncleCount)
	register("eth_getUncleCountByBlockNumber", handleZeroUncleCount)
	register("eth_getUncleByBlockHashAndIndex", handleNullUncle)
	register("eth_getUncleByBlockNumberAndIndex", handleNullUncle)
}

// handleCreateAccessList emits a compatibility-only response: Thor rejects
// non-empty access lists at ingestion, so the access list returned here is
// always empty. gasUsed is hard-coded to 0x0 for the same reason — spec
// Deferred §13.11 covers upgrading this to a real estimate.
func handleCreateAccessList(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
	return map[string]any{
		"accessList": []any{},
		"gasUsed":    hexutil.Uint64(0),
	}, nil
}

// handleGetProof is explicitly unsupported — Thor has no Merkle-Patricia
// trie projection that matches Ethereum's eth_getProof shape.
func handleGetProof(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
	return nil, MethodNotFound("eth_getProof")
}

// handleZeroUncleCount answers the two uncle-count methods; Thor has no
// uncle concept, so this is always 0x0.
func handleZeroUncleCount(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
	return hexutil.Uint64(0), nil
}

// handleNullUncle answers the two uncle-by-index methods; always null.
func handleNullUncle(_ context.Context, _ *Server, _ json.RawMessage) (any, *RPCError) {
	return nil, nil
}
