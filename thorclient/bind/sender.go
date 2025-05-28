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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/tx"
)

// Sender is a transaction sender that builds, signs and/ or sends transactions to the VeChain network.
type Sender struct {
	contract   *Transactor
	vet        *big.Int
	methodName string
	args       []any
	mu         sync.Mutex
	tx         atomic.Pointer[tx.Transaction]
}

// Options to override default transaction parameters when building or sending a transaction.
// See Sender.Build for more details.
type Options struct {
	Gas                  *uint64
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	Expiration           *uint32
	BlockRef             *tx.BlockRef
	Nonce                *uint64
}

func (o *Options) Clone() *Options {
	opts := *o
	return &opts
}

func newSender(contract *Transactor, vet *big.Int, methodName string, args ...any) *Sender {
	return &Sender{
		contract:   contract,
		vet:        vet,
		methodName: methodName,
		args:       args,
	}
}

// Clause builds the transaction clause for the method call with the provided VET amount.
func (s *Sender) Clause() (*tx.Clause, error) {
	clause, err := s.contract.ClauseWithVET(s.vet, s.methodName, s.args...)
	if err != nil {
		return nil, errors.Join(err, s.errorContext())
	}
	return clause, nil
}

// Simulate simulates the method call without sending the transaction to the network.
func (s *Sender) Simulate() (*accounts.CallResult, error) {
	return s.contract.Simulate(s.vet, s.contract.signer.Address(), s.methodName, s.args...)
}

// Build and sign the transaction without sending it to the network.
func (s *Sender) Build(opts *Options) (*tx.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.tx.Load()
	if previous != nil {
		return previous, nil
	}

	if opts == nil {
		opts = &Options{}
	}

	clause, err := s.contract.ClauseWithVET(s.vet, s.methodName, s.args...)
	if err != nil {
		return nil, err
	}
	best, err := s.contract.client.Block("best")
	if err != nil {
		return nil, errors.New("failed to get best block: " + err.Error())
	}
	chainTag, err := s.contract.client.ChainTag()
	if err != nil {
		return nil, errors.New("failed to get chain tag: " + err.Error())
	}
	txType := tx.TypeLegacy
	if best.BaseFeePerGas != nil {
		txType = tx.TypeDynamicFee
	}

	if opts.Gas == nil {
		gas, err := tx.IntrinsicGas(clause)
		if err != nil {
			return nil, errors.New("failed to calculate intrinsic gas: " + err.Error())
		}
		simulation, err := s.Simulate()
		if err != nil {
			return nil, errors.Join(errors.New("simulation failed"), err, s.errorContext())
		}
		gas += simulation.GasUsed
		opts.Gas = &gas
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
	if txType == tx.TypeDynamicFee && opts.MaxFeePerGas == nil {
		opts.MaxFeePerGas = (*big.Int)(best.BaseFeePerGas)
	}

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
	transaction, err = s.contract.signer.SignTransaction(transaction)
	if err != nil {
		return nil, errors.New("failed to sign transaction: " + err.Error())
	}

	return transaction, nil
}

// Send sends the transaction to the network. Does not wait for the receipt.
func (s *Sender) Send(opts *Options) (*tx.Transaction, error) {
	transaction, err := s.Build(opts)
	if err != nil {
		return nil, err
	}

	if _, err = s.contract.client.SendTransaction(transaction); err != nil {
		return nil, err
	}

	s.tx.Store(transaction)

	return transaction, nil
}

// Receipt sends the transaction if it hasn't been sent already and polls for the receipt.
func (s *Sender) Receipt(ctx context.Context, opts *Options) (*transactions.Receipt, *tx.Transaction, error) {
	transaction, err := s.Send(opts)
	if err != nil {
		return nil, nil, err
	}

	id := transaction.ID()

	for {
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("context cancelled while waiting for receipt (method: %s, transaction ID: %s)", s.methodName, id.String())
		default:
			receipt, err := s.contract.client.TransactionReceipt(&id)
			if err != nil || receipt == nil {
				time.Sleep(1 * time.Second) // wait before retrying
				continue
			}
			return receipt, transaction, nil
		}
	}
}

func (s *Sender) errorContext() error {
	method, ok := s.contract.abi.Methods[s.methodName]
	if !ok {
		return errors.New("method not found: " + s.methodName)
	}

	errBuilder := strings.Builder{}
	errBuilder.WriteString("transaction failed")
	errBuilder.WriteString("\nmethod=")
	errBuilder.WriteString(s.methodName)
	errBuilder.WriteString("\nsender=")
	errBuilder.WriteString(s.contract.signer.Address().String())
	errBuilder.WriteString("\nvet=")
	errBuilder.WriteString(s.vet.String())
	for i, arg := range s.args {
		errBuilder.WriteString(fmt.Sprintf("\n%s=", method.Inputs[i].Name))
		errBuilder.WriteString(fmt.Sprintf("%v", arg))
	}

	return errors.New(errBuilder.String())
}

// Senders is a collection of Sender that can send multiple transactions in parallel.
type Senders struct {
	senders []*Sender
	mu      sync.Mutex
}

// Add a new sender to the collection.
func (s *Senders) Add(sender *Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.senders == nil {
		s.senders = make([]*Sender, 0)
	}
	s.senders = append(s.senders, sender)
}

// Send all transactions in parallel and returns the transactions and receipts.
func (s *Senders) Send(ctx context.Context, opts *Options) ([]*transactions.Receipt, []*tx.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	txs := make([]*tx.Transaction, len(s.senders))
	receipts := make([]*transactions.Receipt, len(s.senders))
	errs := make([]error, len(s.senders))

	var wg sync.WaitGroup
	for i, sender := range s.senders {
		wg.Add(1)
		go func(i int, sender *Sender) {
			defer wg.Done()
			receipt, trx, err := sender.Receipt(ctx, opts.Clone())
			if err != nil {
				errs[i] = err
				return
			}
			receipts[i] = receipt
			txs[i] = trx
		}(i, sender)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return receipts, txs, errors.Join(errs...)
		}
	}

	return receipts, txs, nil
}
