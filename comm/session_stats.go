package comm

import (
	"github.com/vechain/thor/thor"
)

// type Traffic struct {
// 	Bytes    uint64
// 	Requests uint64
// 	Errors   uint64
// }

// SessionStats records stats of a p2p session.
type SessionStats struct {
	BestBlockID thor.Bytes32
	TotalScore  uint64
	PeerID      string
	NetAddr     string
	Inbound     bool
	Duration    uint64 // in seconds
}
