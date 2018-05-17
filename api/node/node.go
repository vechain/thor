// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
)

type Node struct {
	nw Network
}

func New(nw Network) *Node {
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

	sub.Path("/network/peers").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))
}
