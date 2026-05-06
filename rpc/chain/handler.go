// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
)

// Handler implements chain metadata JSON-RPC methods.
type Handler struct {
	repo          *chain.Repository
	chainID       uint64
	clientVersion string
}

// New creates a chain Handler.
func New(repo *chain.Repository, chainID uint64, clientVersion string) *Handler {
	return &Handler{repo: repo, chainID: chainID, clientVersion: clientVersion}
}

// Mount registers all chain metadata methods on the dispatcher.
func (h *Handler) Mount(d *rpc.Dispatcher) {
	d.Register("eth_chainId", h.ethChainID)
	d.Register("net_version", h.netVersion)
	d.Register("web3_clientVersion", h.web3ClientVersion)
	d.Register("eth_blockNumber", h.ethBlockNumber)
	d.Register("eth_syncing", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, false) })
	d.Register("eth_accounts", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, []string{}) })
	d.Register("eth_mining", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, false) })
	d.Register("eth_hashrate", func(req rpc.Request) rpc.Response { return rpc.OkResponse(req.ID, "0x0") })
}

func (h *Handler) ethChainID(req rpc.Request) rpc.Response {
	return rpc.OkResponse(req.ID, hexutil.Uint64(h.chainID))
}

func (h *Handler) netVersion(req rpc.Request) rpc.Response {
	return rpc.OkResponse(req.ID, strconv.FormatUint(h.chainID, 10))
}

func (h *Handler) web3ClientVersion(req rpc.Request) rpc.Response {
	return rpc.OkResponse(req.ID, "Thor/"+h.clientVersion)
}

func (h *Handler) ethBlockNumber(req rpc.Request) rpc.Response {
	num := h.repo.BestBlockSummary().Header.Number()
	return rpc.OkResponse(req.ID, hexutil.Uint64(num))
}
