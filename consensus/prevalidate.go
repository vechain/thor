// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// PrevalidateStateless runs the block checks that need no state, no ecrecover
// and no parent, for gating untrusted blocks before they enter the future block
// cache.
//
// ceilingGasLimit must come from a trusted on-chain header: a future block's own
// gas limit is never validated by consensus, so an unpinned one can be set to
// 2^64-1 and make the intrinsic gas sum below vacuously true.
func (c *Consensus) PrevalidateStateless(blk *block.Block, ceilingGasLimit uint64) error {
	header := blk.Header()

	if header.GasLimit() > ceilingGasLimit {
		return consensusError(fmt.Sprintf("block gas limit above ceiling: ceiling %v, current %v", ceilingGasLimit, header.GasLimit()))
	}

	signature := header.Signature()
	if thor.IsForked(header.Number(), c.forkConfig.VIP214) {
		if len(signature) != block.ComplexSigSize {
			return consensusError(fmt.Sprintf("block signature length invalid: want %d have %v", block.ComplexSigSize, len(signature)))
		}
		// Legit alpha is parent.StateRoot() or parent.Beta(), always 32 bytes.
		// Decoding bounds it only by the 10MB message size.
		if len(header.Alpha()) != 32 {
			return consensusError(fmt.Sprintf("block alpha length invalid: want 32 have %v", len(header.Alpha())))
		}
	} else {
		if len(header.Alpha()) > 0 {
			return consensusError("invalid block, alpha should be empty before VIP214")
		}
		if len(signature) != 65 {
			return consensusError(fmt.Sprintf("block signature length invalid: want 65 have %v", len(signature)))
		}
	}

	var totalIntrinsic uint64
	for _, tr := range blk.Transactions() {
		gas, err := tr.IntrinsicGas()
		if err != nil {
			return consensusError(fmt.Sprintf("tx intrinsic gas unavailable: %v", err))
		}

		if tr.Gas() < gas {
			return consensusError(fmt.Sprintf("tx gas below intrinsic: gas %v, intrinsic %v", tr.Gas(), gas))
		}

		// Overflow-free totalIntrinsic+gas > GasLimit; totalIntrinsic <= GasLimit on entry.
		if gas > header.GasLimit()-totalIntrinsic {
			return consensusError(fmt.Sprintf("block intrinsic gas exceeds limit: limit %v", header.GasLimit()))
		}
		totalIntrinsic += gas

		// IntrinsicGas does not price the signature, so padding is free weight.
		if err := tr.ValidateSignatureLength(); err != nil {
			return consensusError(fmt.Sprintf("tx signature length invalid: %v", err))
		}

		if tr.ChainTag() != c.repo.ChainTag() {
			return consensusError(fmt.Sprintf("tx chain tag mismatch: want %v, have %v", c.repo.ChainTag(), tr.ChainTag()))
		}

		// IntrinsicGas does not price reserved fields and their size is unbounded
		// at decode; consensus rejects any unused slot anyway (validator.go:212).
		if err := tr.TestFeatures(header.TxsFeatures()); err != nil {
			return consensusError(fmt.Sprintf("invalid tx: %v", err))
		}
	}

	return nil
}
