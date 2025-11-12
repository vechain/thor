package testchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// MintFromABI creates a transaction with a single clause using the provided ABI, address, method and args
func (c *Chain) MintFromABI(
	account genesis.DevAccount,
	addr thor.Address,
	abi *abi.ABI,
	vet *big.Int,
	method string,
	args ...any,
) error {
	m, ok := abi.MethodByName(method)
	if !ok {
		return fmt.Errorf("unable to find method %s in ABI", method)
	}
	callData, err := m.EncodeInput(args...)
	if err != nil {
		return fmt.Errorf("unable to encode method %s input: %w", method, err)
	}
	clause := tx.NewClause(&addr).WithData(callData).WithValue(vet)

	return c.MintClauses(account, []*tx.Clause{clause})
}

// MintClauses creates a transaction with the provided clauses and mints a block containing that transaction.
func (c *Chain) MintClauses(account genesis.DevAccount, clauses []*tx.Clause) error {
	builder := new(tx.Builder).GasPriceCoef(255).
		BlockRef(tx.NewBlockRef(c.Repo().BestBlockSummary().Header.Number())).
		Expiration(1000).
		ChainTag(c.Repo().ChainTag()).
		Gas(10e6).
		Nonce(datagen.RandUint64())

	for _, clause := range clauses {
		builder.Clause(clause)
	}

	tx := builder.Build()
	signature, err := crypto.Sign(tx.SigningHash().Bytes(), account.PrivateKey)
	if err != nil {
		return fmt.Errorf("unable to sign tx: %w", err)
	}
	tx = tx.WithSignature(signature)

	return c.MintBlock(tx)
}

// AddValidators creates an `addValidation` staker transaction for each validator and mints a block containing those transactions.
func (c *Chain) AddValidators() error {
	method, ok := builtin.Staker.ABI.MethodByName("addValidation")
	if !ok {
		return errors.New("unable to find addValidation method in staker ABI")
	}

	stakerTxs := make([]*tx.Transaction, len(c.validators))
	for i, val := range c.validators {
		callData, err := method.EncodeInput(val.Address, thor.LowStakingPeriod())
		if err != nil {
			return fmt.Errorf("unable to encode addValidation input: %w", err)
		}
		clause := tx.NewClause(&builtin.Staker.Address).WithData(callData).WithValue(staker.MinStake)

		trx := new(tx.Builder).
			GasPriceCoef(255).
			BlockRef(tx.NewBlockRef(c.Repo().BestBlockSummary().Header.Number())).
			Expiration(1000).
			ChainTag(c.Repo().ChainTag()).
			Gas(10e6).
			Nonce(datagen.RandUint64()).
			Clause(clause).
			Build()

		trx = tx.MustSign(trx, val.PrivateKey)
		stakerTxs[i] = trx
	}

	return c.MintBlock(stakerTxs...)
}
