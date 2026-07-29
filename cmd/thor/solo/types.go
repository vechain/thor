// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"github.com/vechain/thor/v2/comm"
)

// soloSyncedCh is a pre-closed channel returned by Communicator.Synced.
// Solo mode is by definition already synced — there is no peer network to
// catch up with.
var soloSyncedCh = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// Communicator in solo is a fake one just for api handler.
type Communicator struct{}

// PeersStats returns nil solo doesn't join p2p network.
func (comm *Communicator) PeersStats() []*comm.PeerStats {
	return nil
}

// Synced returns a pre-closed channel: solo mode has no peers to sync with,
// so the node is always considered synced.
func (comm *Communicator) Synced() <-chan struct{} {
	return soloSyncedCh
}
