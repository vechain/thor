package comm

import (
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/vechain/thor/thor"
	set "gopkg.in/fatih/set.v0"
)

const (
	maxKnownTxs      = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks   = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
	handshakeTimeout = 5 * time.Second
)

type peer struct {
	*p2p.Peer

	knownTxs    *set.Set
	knownBlocks *set.Set
}

func (p *peer) MarkTransaction(id thor.Hash) {
	for p.knownTxs.Size() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(id)
}

func (p *peer) MarkBlock(id thor.Hash) {
	for p.knownBlocks.Size() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(id)
}
