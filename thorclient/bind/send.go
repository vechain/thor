// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

// TxOptions to override default transaction parameters when building or sending a transaction.
type TxOptions struct {
	// Gas sets the gas limit for the transaction.
	Gas *uint64
	// MaxFeePerGas sets the maximum fee per gas unit.
	MaxFeePerGas *big.Int
	// MaxPriorityFeePerGas sets the maximum priority fee per gas unit.
	MaxPriorityFeePerGas *big.Int
	// Expiration sets the number of blocks until the transaction expires.
	Expiration *uint32
	// BlockRef sets the block reference for the transaction.
	BlockRef *tx.BlockRef
	// Nonce sets the transaction nonce.
	Nonce *uint64
	// DependsOn is an optional transaction ID that this transaction depends on.
	DependsOn *thor.Bytes32
}

// SendBuilder is the concrete implementation of SendBuilder.
type SendBuilder struct {
	op     *MethodBuilder
	signer Signer
	opts   *TxOptions
}

// WithSigner implements SendBuilder.WithSigner.
func (b *SendBuilder) WithSigner(signer Signer) *SendBuilder {
	b.signer = signer
	return b
}

// WithOptions implements SendBuilder.WithOptions.
func (b *SendBuilder) WithOptions(opts *TxOptions) *SendBuilder {
	b.opts = opts
	return b
}

// Submit implements SendBuilder.IssueTx.
func (b *SendBuilder) Submit() (*tx.Transaction, error) {
	if b.signer == nil {
		return nil, errors.New("signer not set")
	}

	// Build the clause
	clause, err := b.op.Clause()
	if err != nil {
		return nil, err
	}

	// Copy the options so that the defaults set below never leak into the
	// caller's TxOptions, and a reused builder gets a fresh nonce and blockRef.
	var opts TxOptions
	if b.opts != nil {
		opts = *b.opts
	}

	if opts.Gas == nil {
		gas, _, err := estimateGas(b.op.contract.client, b.signer.Address(), clause)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate gas (%s): %w", b.op.String(), err)
		}
		opts.Gas = &gas
	}

	transaction, err := buildTx(b.op.contract.client, clause, opts)
	if err != nil {
		return nil, err
	}
	transaction, err = b.signer.SignTransaction(transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	if _, err = b.op.contract.client.SendTransaction(transaction); err != nil {
		return nil, fmt.Errorf("failed to send transaction (%s): %w", b.op.String(), err)
	}

	return transaction, nil
}

// buildTx builds an unsigned tx for clause on top of the best block. opts.Gas
// must be set. Every other unset option gets a default. The tx is dynamic-fee
// when the best block has a base fee, and legacy otherwise.
func buildTx(client *thorclient.Client, clause *tx.Clause, opts TxOptions) (*tx.Transaction, error) {
	best, err := client.Block("best")
	if err != nil {
		return nil, fmt.Errorf("failed to get best block: %w", err)
	}
	chainTag, err := client.ChainTag()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain tag: %w", err)
	}

	txType := tx.TypeLegacy
	if best.BaseFeePerGas != nil {
		txType = tx.TypeDynamicFee
	}

	if opts.Expiration == nil {
		expiration := uint32(60) // 60 blocks ~10 minutes
		opts.Expiration = &expiration
	}
	if opts.BlockRef == nil {
		ref := tx.NewBlockRef(best.Number)
		opts.BlockRef = &ref
	}
	if opts.Nonce == nil {
		nonce := datagen.RandUint64()
		opts.Nonce = &nonce
	}

	builder := tx.NewBuilder(txType).
		Clause(clause).
		Gas(*opts.Gas).
		ChainTag(chainTag).
		Expiration(*opts.Expiration).
		BlockRef(*opts.BlockRef).
		Nonce(*opts.Nonce).
		DependsOn(opts.DependsOn)

	switch txType {
	case tx.TypeLegacy:
		builder.GasPriceCoef(0)
	case tx.TypeDynamicFee:
		maxFee := opts.MaxFeePerGas
		if maxFee == nil {
			maxFee = defaultMaxFeePerGas((*big.Int)(best.BaseFeePerGas), opts.MaxPriorityFeePerGas)
		}
		builder.MaxFeePerGas(maxFee)
		if opts.MaxPriorityFeePerGas != nil {
			builder.MaxPriorityFeePerGas(opts.MaxPriorityFeePerGas)
		}
	}

	return builder.Build(), nil
}

// defaultMaxFeePerGas returns 2*baseFee + tip. The txpool and the packer price a
// tx against the next block's base fee, which can be up to 12.5% higher per
// block. Doubling the base fee covers several increases, and adding the tip
// keeps it fully payable. The tx pays min(maxFee, baseFee+tip), so the extra
// headroom costs nothing.
func defaultMaxFeePerGas(baseFee, tip *big.Int) *big.Int {
	maxFee := new(big.Int).Lsh(baseFee, 1)
	if tip != nil {
		maxFee.Add(maxFee, tip)
	}
	return maxFee
}

// SubmitAndConfirm implements SendBuilder.Receipt.
func (b *SendBuilder) SubmitAndConfirm(ctx context.Context) (*api.Receipt, *tx.Transaction, error) {
	transaction, err := b.Submit()
	if err != nil {
		return nil, nil, err
	}

	id := transaction.ID()
	for {
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("context cancelled while waiting for receipt (context: %s, transaction ID: %s)", b.op.String(), id.String())
		default:
			receipt, err := b.op.contract.client.TransactionReceipt(&id)
			if err != nil || receipt == nil {
				time.Sleep(1 * time.Second) // wait before retrying
				continue
			}
			return receipt, transaction, nil
		}
	}
}
