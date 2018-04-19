package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
)

type Network interface {
	SessionCount() int
}
type Node struct {
	nw Network
}

func New(nw Network) *Node {
	return &Node{
		nw,
	}
}

func (n *Node) peerCount() int {
	return n.nw.SessionCount()
}

func (n *Node) handleNetwork(w http.ResponseWriter, req *http.Request) error {
	return utils.WriteJSON(w, map[string]int{"peerCount": n.peerCount()})
}

func (n *Node) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))
}
