// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func (c *Core) ForkTransactions(best *chain.BlockSummary) ([]*tx.Transaction, error) {
	txs := make([]*tx.Transaction, 0)

	newState := c.stater.NewState(best.Root())
	currentBGP, err := builtin.Params.Native(newState).Get(thor.KeyLegacyTxBaseGasPrice)
	if err != nil {
		return nil, err
	}
	if currentBGP.Cmp(baseGasPrice) != 0 {
		logger.Info("SOLO FORK: setting base gas price", "block", best.Header.Number())
		baseGasPriceTX, err := c.baseGasPriceTX()
		if err != nil {
			return nil, errors.WithMessage(err, "create base gas price transaction")
		}
		txs = append(txs, baseGasPriceTX)
	}

	// This sets up the authority contract will all dev accounts and sets the max block proposers to 14
	if best.Header.Number()+1 == c.forkConfig.HAYABUSA-1 {
		logger.Info("SOLO FORK: setting up PRE-HAYABUSA transactions", "block", best.Header.Number())
		mbpTx, err := c.mbpTransaction()
		if err != nil {
			return nil, errors.WithMessage(err, "create mbp transaction")
		}
		txs = append(txs, mbpTx)
		authorityTx, err := c.addAuthoritiesTx()
		if err != nil {
			return nil, errors.WithMessage(err, "create authority transactions")
		}
		txs = append(txs, authorityTx)
	}

	if best.Header.Number()+1 == c.forkConfig.HAYABUSA {
		logger.Info("SOLO FORK: setting up HAYABUSA transaction", "block", best.Header.Number())
		stakeTXs, err := c.stakeTransactions()
		if err != nil {
			return nil, errors.WithMessage(err, "create stake transaction")
		}
		txs = append(txs, stakeTXs...)
	}
	return txs, nil
}

func (c *Core) baseGasPriceTX() (*tx.Transaction, error) {
	method, found := builtin.Params.ABI.MethodByName("set")
	if !found {
		return nil, errors.New("Params ABI: set method not found")
	}

	data, err := method.EncodeInput(thor.KeyLegacyTxBaseGasPrice, baseGasPrice)
	if err != nil {
		return nil, errors.WithMessage(err, "encode input for set method")
	}

	clause := tx.NewClause(&builtin.Params.Address).WithData(data)
	return c.createTransaction([]*tx.Clause{clause}, genesis.DevAccounts()[0])
}

func (c *Core) mbpTransaction() (*tx.Transaction, error) {
	method, ok := builtin.Params.ABI.MethodByName("set")
	if !ok {
		return nil, errors.New("method set not found in Params ABI")
	}
	data, err := method.EncodeInput(thor.KeyMaxBlockProposers, big.NewInt(14))
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(&builtin.Params.Address).WithData(data)
	return c.createTransaction([]*tx.Clause{clause}, genesis.DevAccounts()[0])
}

func (c *Core) addAuthoritiesTx() (*tx.Transaction, error) {
	method, ok := builtin.Authority.ABI.MethodByName("add")
	if !ok {
		return nil, errors.New("method set not found in Params ABI")
	}

	clauses := make([]*tx.Clause, 0)
	for i, authority := range genesis.DevAccounts()[1:] {
		id := fmt.Sprintf("Solo Block Signer %d", i)
		data, err := method.EncodeInput(authority.Address, authority.Address, thor.BytesToBytes32([]byte(id)))
		if err != nil {
			return nil, err
		}
		clause := tx.NewClause(&builtin.Authority.Address).WithData(data)
		clauses = append(clauses, clause)
	}
	return c.createTransaction(clauses, genesis.DevAccounts()[0])
}

func (c *Core) stakeTransactions() ([]*tx.Transaction, error) {
	method, ok := builtin.Staker.ABI.MethodByName("addValidator")
	if !ok {
		return nil, errors.New("method addValidator not found in Staker ABI")
	}
	transactions := make([]*tx.Transaction, 0)
	for _, account := range genesis.DevAccounts() {
		data, err := method.EncodeInput(account.Address, staker.HighStakingPeriod)
		if err != nil {
			return nil, err
		}

		clause := tx.NewClause(&builtin.Staker.Address).
			WithData(data).
			WithValue(staker.MinStake)

		tx, err := c.createTransaction([]*tx.Clause{clause}, account)
		if err != nil {
			return nil, errors.WithMessage(err, "create stake transaction")
		}
		transactions = append(transactions, tx)
	}
	return transactions, nil
}

func (c *Core) createTransaction(clauses []*tx.Clause, signer genesis.DevAccount) (*tx.Transaction, error) {
	best := c.repo.BestBlockSummary()

	var builder *tx.Builder
	if best.Header.BaseFee() != nil {
		maxFee := big.NewInt(0).Set(best.Header.BaseFee())
		maxFee.Mul(maxFee, big.NewInt(110))
		maxFee.Div(maxFee, big.NewInt(100))

		builder = tx.NewBuilder(tx.TypeDynamicFee).
			MaxFeePerGas(maxFee)
	} else {
		builder = tx.NewBuilder(tx.TypeLegacy)
	}

	for _, clause := range clauses {
		builder.Clause(clause)
	}

	trx := builder.BlockRef(tx.NewBlockRef(0)).
		ChainTag(c.repo.ChainTag()).
		Expiration(math.MaxUint32).
		Nonce(datagen.RandUint64()).
		Gas(1_000_000).
		Build()

	return tx.Sign(trx, signer.PrivateKey)
}
