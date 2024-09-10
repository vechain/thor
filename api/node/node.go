// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/utils"
)

const MountPath = "/node"

type Node struct {
	nw Network
}

func New(nw Network) utils.APIServer {
	return &Node{
		nw,
	}
}

func (n *Node) PeersStats() []*PeerStats {
	return ConvertPeersStats(n.nw.PeersStats())
}

func (n *Node) handleNetwork(w http.ResponseWriter, req *http.Request) error {
	return utils.WriteJSON(w, n.PeersStats())
}

func (n *Node) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network/peers").
		Methods(http.MethodGet).
		Name("node_get_peers").
		HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))
}

func (n *Node) MountDefaultPath(router *mux.Router) {
	n.Mount(router, MountPath)
}
