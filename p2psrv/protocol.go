package p2psrv

import "github.com/ethereum/go-ethereum/p2p"

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	p2p.Protocol

	DiscTopic string
}
