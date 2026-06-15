// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
)

type Syncer interface {
	Synced() <-chan struct{}
	HighestPeerBlock() uint32
}

type syncingStatus struct {
	StartingBlock hexutil.Uint64 `json:"startingBlock"`
	CurrentBlock  hexutil.Uint64 `json:"currentBlock"`
	HighestBlock  hexutil.Uint64 `json:"highestBlock"`
}

// Handler implements chain metadata JSON-RPC methods.
type Handler struct {
	repo          *chain.Repository
	clientVersion string
	syncer        Syncer
	startingBlock uint32
}

// New creates a chain Handler.
func New(repo *chain.Repository, clientVersion string, syncer Syncer) *Handler {
	return &Handler{
		repo:          repo,
		clientVersion: clientVersion,
		syncer:        syncer,
		startingBlock: repo.BestBlockSummary().Header.Number(),
	}
}

// Mount registers all chain metadata methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_chainId", h.ethChainID)
	s.Register("net_version", h.netVersion)
	s.Register("net_listening", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, true) })
	s.Register(
		"net_peerCount",
		func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, hexutil.Uint64(0)) },
	) // VeChain PoA has no mining peers
	s.Register("web3_clientVersion", h.web3ClientVersion)
	s.Register("eth_blockNumber", h.ethBlockNumber)
	s.Register("eth_coinbase", h.ethCoinbase)
	s.Register("eth_syncing", h.ethSyncing)
	s.Register("eth_accounts", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, []string{}) })
	s.Register("eth_mining", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, false) })
	s.Register("eth_hashrate", func(req jsonrpc.Request) jsonrpc.Response { return jsonrpc.OkResponse(req.ID, hexutil.Uint64(0)) })
}

func (h *Handler) ethChainID(req jsonrpc.Request) jsonrpc.Response {
	return jsonrpc.OkResponse(req.ID, hexutil.Uint64(h.repo.ChainID()))
}

func (h *Handler) netVersion(req jsonrpc.Request) jsonrpc.Response {
	return jsonrpc.OkResponse(req.ID, strconv.FormatUint(h.repo.ChainID(), 10))
}

func (h *Handler) web3ClientVersion(req jsonrpc.Request) jsonrpc.Response {
	return jsonrpc.OkResponse(req.ID, "Thor/"+h.clientVersion)
}

func (h *Handler) ethBlockNumber(req jsonrpc.Request) jsonrpc.Response {
	num := h.repo.BestBlockSummary().Header.Number()
	return jsonrpc.OkResponse(req.ID, hexutil.Uint64(num))
}

func (h *Handler) ethCoinbase(req jsonrpc.Request) jsonrpc.Response {
	return jsonrpc.OkResponse(req.ID, common.Address{})
}

func (h *Handler) ethSyncing(req jsonrpc.Request) jsonrpc.Response {
	select {
	case <-h.syncer.Synced():
		return jsonrpc.OkResponse(req.ID, false)
	default:
	}
	current := h.repo.BestBlockSummary().Header.Number()
	highest := h.syncer.HighestPeerBlock()
	if current > highest {
		highest = current
	}
	return jsonrpc.OkResponse(req.ID, syncingStatus{
		StartingBlock: hexutil.Uint64(h.startingBlock),
		CurrentBlock:  hexutil.Uint64(current),
		HighestBlock:  hexutil.Uint64(highest),
	})
}
