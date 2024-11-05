// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/thor"
)

// Communicator in solo is a fake one just for api handler.
type Communicator struct {
}

// PeersStats returns nil solo doesn't join p2p network.
func (comm *Communicator) PeersStats() []*comm.PeerStats {
	return nil
}

// Synced returns a channel that is already synced.
func (comm *Communicator) Synced() <-chan struct{} {
	syncedChan := make(chan struct{}, 1)
	syncedChan <- struct{}{}
	return syncedChan
}

// BFTEngine is a fake bft engine for solo.
type BFTEngine struct {
	finalized thor.Bytes32
	justified thor.Bytes32
}

func (engine *BFTEngine) Finalized() thor.Bytes32 {
	return engine.finalized
}

func (engine *BFTEngine) Justified() (thor.Bytes32, error) {
	return engine.justified, nil
}

func NewBFTEngine(repo *chain.Repository) *BFTEngine {
	return &BFTEngine{
		finalized: repo.GenesisBlock().Header().ID(),
		justified: repo.GenesisBlock().Header().ID(),
	}
}
