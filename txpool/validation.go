// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func validateTransaction(tr *tx.Transaction, repo *chain.Repository, head *chain.BlockSummary, forkConfig *thor.ForkConfig) error {
	if tr.ChainTag() != repo.ChainTag() {
		return badTxError{"chain tag mismatch"}
	}
	if tr.Size() > maxTxSize {
		return txRejectedError{"size too large"}
	}
	if head.Header.Number() < forkConfig.GALACTICA {
		// Pre-Galactica, only support legacy tx
		if tr.Type() != tx.TypeLegacy {
			return tx.ErrTxTypeNotSupported
		}
	}
	if err := tr.TestFeatures(head.Header.TxsFeatures()); err != nil {
		return txRejectedError{err.Error()}
	}

	return nil
}
