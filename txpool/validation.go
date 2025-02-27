// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
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
		// Pre-Galactica, only support legacy tx
		if tr.Type() != tx.TypeLegacy {
			return tx.ErrTxTypeNotSupported
		}
	} else {
		// Post-Galactica
		if tr.MaxFeePerGas() == nil {
			return txRejectedError{"max fee per gas is required"}
		}
		if tr.MaxPriorityFeePerGas() == nil {
			return txRejectedError{"max priority fee per gas is required"}
		}
		if tr.MaxFeePerGas().Cmp(tr.MaxPriorityFeePerGas()) < 0 {
			return txRejectedError{fmt.Sprintf("max fee per gas (%v) must be greater than max priority fee per gas (%v)\n", tr.MaxFeePerGas(), tr.MaxPriorityFeePerGas())}
		}

		if tr.MaxFeePerGas().Sign() < 0 {
			return txRejectedError{"max fee per gas must be positive"}
		}
		if tr.MaxPriorityFeePerGas().Sign() < 0 {
			return txRejectedError{"max priority fee per gas must be positive"}
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

func ValidateTransactionWithState(tr *tx.Transaction, header *block.Header, forkConfig *thor.ForkConfig, state *state.State) error {
	if header.Number() >= forkConfig.GALACTICA {
		baseGasPrice, err := builtin.Params.Native(state).Get(thor.KeyBaseGasPrice)
		if err != nil {
			return err
		}
		if err := fork.ValidateGalacticaTxFee(tr, header.BaseFee(), baseGasPrice); err != nil {
			return txRejectedError{err.Error()}
		}
	}

	return nil
}
