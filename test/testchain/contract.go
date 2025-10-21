// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Contract struct {
	chain *Chain
	abi   *abi.ABI
	addr  thor.Address
	acc   genesis.DevAccount
}

func NewContract(chain *Chain, acc genesis.DevAccount, addr thor.Address, abi *abi.ABI) *Contract {
	return &Contract{
		chain: chain,
		abi:   abi,
		addr:  addr,
		acc:   acc,
	}
}

func (c *Contract) Attach(acc genesis.DevAccount) *Contract {
	contract := *c
	contract.acc = acc
	return &contract
}

// Call calls a contract method and returns the result.
func (c *Contract) Call(method string, args ...any) ([]byte, error) {
	clause, err := c.BuildClause(method, args...)
	if err != nil {
		return nil, err
	}
	trx := new(tx.Builder).Clause(clause).Build()
	output, _, err := c.chain.ClauseCall(c.acc, trx, 0)
	return output, err
}

// CallInto calls a contract method and decodes the result into the result argument.
func (c *Contract) CallInto(method string, result any, args ...any) error {
	data, err := c.Call(method, args...)
	if err != nil {
		return err
	}
	methodABI, ok := c.abi.MethodByName(method)
	if !ok {
		return errors.New("method not found")
	}
	return methodABI.DecodeOutput(data, result)
}

func (c *Contract) BuildClause(method string, args ...any) (*tx.Clause, error) {
	methodABI, ok := c.abi.MethodByName(method)
	if !ok {
		return nil, errors.New("method not found")
	}
	data, err := methodABI.EncodeInput(args...)
	if err != nil {
		return nil, err
	}
	return tx.NewClause(&c.addr).WithData(data), nil
}

func (c *Contract) MintTransaction(method string, vet *big.Int, args ...any) error {
	clause, err := c.BuildClause(method, args...)
	if err != nil {
		return err
	}
	if vet != nil {
		clause = clause.WithValue(vet)
	}
	return c.chain.MintClauses(c.acc, []*tx.Clause{clause})
}

func (c *Contract) BuildTransaction(method string, vet *big.Int, args ...any) (*tx.Transaction, error) {
	clause, err := c.BuildClause(method, args...)
	if err != nil {
		return nil, err
	}
	clause = clause.WithValue(vet)
	trx := new(tx.Builder).
		Clause(clause).
		Gas(1_000_000).
		ChainTag(c.chain.ChainTag()).
		Nonce(datagen.RandUint64()).
		Expiration(1_000).
		BlockRef(tx.NewBlockRef(c.chain.Repo().BestBlockSummary().Header.Number())).
		Build()

	trx = tx.MustSign(trx, c.acc.PrivateKey)

	return trx, nil
}
