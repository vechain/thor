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

func (n *Node) SessionsStats() []*SessionStats {
	return ConvertSessionStats(n.nw.SessionsStats())
}

func (n *Node) handleNetwork(w http.ResponseWriter, req *http.Request) error {
	return utils.WriteJSON(w, n.SessionsStats())
}

func (n *Node) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network/sessions").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))
}
