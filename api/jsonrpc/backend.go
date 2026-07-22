// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type backend struct {
	repo       *chain.Repository
	stater     *state.Stater
	bft        bft.Committer
	forkConfig *thor.ForkConfig
}

// stateForRevision reuses the REST revision resolver so JSON-RPC and REST see the
// same chain state. GetSummaryAndState takes a *restutil.Revision, so parse first.
func (b *backend) stateForRevision(revStr string) (*chain.BlockSummary, *state.State, error) {
	rev, err := restutil.ParseRevision(revStr, false)
	if err != nil {
		return nil, nil, err
	}
	return restutil.GetSummaryAndState(rev, b.repo, b.bft, b.stater, b.forkConfig)
}
