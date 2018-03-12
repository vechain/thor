package p2psrv

import "github.com/ethereum/go-ethereum/p2p/discover"

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	// Name should contain the official protocol name,
	// often a three-letter word.
	Name string

	// Version should contain the version number of the protocol.
	Version uint32

	// Length should contain the number of message codes used
	// by the protocol.
	Length uint64

	// NodeInfo is an optional helper method to retrieve protocol specific metadata
	// about the host node.
	NodeInfo func() interface{}

	// PeerInfo is an optional helper method to retrieve protocol specific metadata
	// about a certain peer in the network. If an info retrieval function is set,
	// but returns nil, it is assumed that the protocol handshake is still running.
	PeerInfo func(id discover.NodeID) interface{}

	HandleRequest HandleRequest
}
