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

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

// SendBuilder is the interface for write operations.
type SendBuilder interface {
	// WithSigner sets the signer for the transaction.
	WithSigner(signer Signer) SendBuilder

	// WithOptions sets the transaction options.
	WithOptions(opts *TxOptions) SendBuilder

	// IssueTx sends the transaction without waiting for receipt.
	IssueTx() (*tx.Transaction, error)

	// Receipt sends the transaction and waits for the receipt.
	Receipt(ctx context.Context) (*transactions.Receipt, *tx.Transaction, error)
}

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
}

// sendBuilder is the concrete implementation of SendBuilder.
type sendBuilder struct {
	op     *methodBuilder
	signer Signer
	opts   *TxOptions
}

// WithSigner implements SendBuilder.WithSigner.
func (b *sendBuilder) WithSigner(signer Signer) SendBuilder {
	b.signer = signer
	return b
}

// WithOptions implements SendBuilder.WithOptions.
func (b *sendBuilder) WithOptions(opts *TxOptions) SendBuilder {
	b.opts = opts
	return b
}

// IssueTx implements SendBuilder.IssueTx.
func (b *sendBuilder) IssueTx() (*tx.Transaction, error) {
	if b.signer == nil {
		return nil, errors.New("signer not set")
	}

	// Build the clause
	clause, err := b.op.Clause()
	if err != nil {
		return nil, err
	}

	// Build the transaction
	best, err := b.op.contract.client.Block("best")
	if err != nil {
		return nil, fmt.Errorf("failed to get best block: %w", err)
	}
	chainTag, err := b.op.contract.client.ChainTag()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain tag: %w", err)
	}

	txType := tx.TypeLegacy
	if best.BaseFeePerGas != nil {
		txType = tx.TypeDynamicFee
	}

	opts := b.opts
	if opts == nil {
		opts = &TxOptions{}
	}

	// Calculate gas if not provided
	if opts.Gas == nil {
		gas, err := tx.IntrinsicGas(clause)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate intrinsic gas: %w", err)
		}
		caller := b.signer.Address()
		simulation, err := b.op.contract.client.InspectClauses(
			&accounts.BatchCallData{
				Caller:  &caller,
				Clauses: []accounts.Clause{{To: b.op.contract.addr, Data: hexutil.Encode(clause.Data()), Value: (*math.HexOrDecimal256)(clause.Value())}},
			}, thorclient.Revision("best"))
		if err != nil {
			return nil, fmt.Errorf("simulation failed: %w", err)
		}
		if len(simulation) != 1 {
			return nil, fmt.Errorf("expected 1 simulation result, got %d", len(simulation))
		}
		gas += simulation[0].GasUsed
		opts.Gas = &gas
	}

	// Set default options if not provided
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
	if txType == tx.TypeDynamicFee && opts.MaxFeePerGas == nil {
		opts.MaxFeePerGas = (*big.Int)(best.BaseFeePerGas)
	}

	// Build the transaction
	builder := new(tx.Builder).
		Clause(clause).
		Gas(*opts.Gas).
		ChainTag(chainTag).
		Expiration(*opts.Expiration).
		BlockRef(*opts.BlockRef).
		Nonce(*opts.Nonce)

	switch txType {
	case tx.TypeLegacy:
		builder.GasPriceCoef(0)
	case tx.TypeDynamicFee:
		builder.MaxFeePerGas(opts.MaxFeePerGas)
		if opts.MaxPriorityFeePerGas != nil {
			builder.MaxPriorityFeePerGas(opts.MaxPriorityFeePerGas)
		}
	}

	transaction := builder.Build()
	transaction, err = b.signer.SignTransaction(transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	if _, err = b.op.contract.client.SendTransaction(transaction); err != nil {
		return nil, err
	}

	return transaction, nil
}

// Receipt implements SendBuilder.Receipt.
func (b *sendBuilder) Receipt(ctx context.Context) (*transactions.Receipt, *tx.Transaction, error) {
	transaction, err := b.IssueTx()
	if err != nil {
		return nil, nil, err
	}

	id := transaction.ID()
	for {
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("context cancelled while waiting for receipt (method: %s, transaction ID: %s)", b.op.method, id.String())
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
