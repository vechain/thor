package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/comm"
)

type Node struct {
	comm *comm.Communicator
}

func New(comm *comm.Communicator) *Node {
	return &Node{
		comm,
	}
}

func (n *Node) peerCount() int {
	return n.comm.SessionCount()
}

func (n *Node) handleNetwork(w http.ResponseWriter, req *http.Request) error {
	return utils.WriteJSON(w, map[string]int{"peerCount": n.peerCount()})
}

func (n *Node) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network").Methods("Get").HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))
}
