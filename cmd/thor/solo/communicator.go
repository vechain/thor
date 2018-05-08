package solo

import "github.com/vechain/thor/comm"

// Communicator in solo is a fake one just for api handler
type Communicator struct {
}

// SessionsStats returns nil solo doesn't join p2p network
func (comm Communicator) SessionsStats() []*comm.SessionStats {
	return nil
}
