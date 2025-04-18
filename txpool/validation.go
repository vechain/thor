// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ValidateTransactionWithState(tr *tx.Transaction, header *block.Header, forkConfig *thor.ForkConfig, state *state.State) error {
	if header.Number() >= forkConfig.GALACTICA {
		legacyTxBaseGasPrice, err := builtin.Params.Native(state).Get(thor.KeyLegacyTxBaseGasPrice)
		if err != nil {
			return err
		}
		if err := fork.ValidateGalacticaTxFee(tr, header.BaseFee(), legacyTxBaseGasPrice); err != nil {
			return txRejectedError{err.Error()}
		}
	}

	return nil
}
