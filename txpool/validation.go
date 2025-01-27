// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ValidateTransaction(tr *tx.Transaction, repo *chain.Repository, head *chain.BlockSummary, forkConfig *thor.ForkConfig) error {
	if tr.ChainTag() != repo.ChainTag() {
		return badTxError{"chain tag mismatch"}
	}
	if tr.Size() > maxTxSize {
		return txRejectedError{"size too large"}
	}
	if head.Header.Number() < forkConfig.GALACTICA {
		if tr.Type() != tx.LegacyTxType {
			return tx.ErrTxTypeNotSupported
		}
	}
	if err := tr.TestFeatures(head.Header.TxsFeatures()); err != nil {
		return txRejectedError{err.Error()}
	}
	// Sanity check for extremely large numbers (supported by RLP)
	if tr.MaxFeePerGas().BitLen() > 256 {
		return tx.ErrMaxFeeVeryHigh
	}
	if tr.MaxPriorityFeePerGas().BitLen() > 256 {
		return tx.ErrMaxPriorityFeeVeryHigh
	}

	return nil
}

func ValidateTransactionWithState(tr *tx.Transaction, head *chain.BlockSummary, forkConfig *thor.ForkConfig, state *state.State) error {
	if head.Header.Number() >= forkConfig.GALACTICA {
		baseGasPrice, err := builtin.Params.Native(state).Get(thor.KeyBaseGasPrice)
		if err != nil {
			return txRejectedError{err.Error()}
		}
		galacticaItems := fork.GalacticaTxGasPriceAdapater(tr, baseGasPrice)
		if galacticaItems.MaxFee.Cmp(head.Header.BaseFee()) < 0 {
			return txRejectedError{"max fee per gas too low to cover for base fee"}
		}
	}

	return nil
}
