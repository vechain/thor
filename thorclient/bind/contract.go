// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"bytes"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

// Contract is the main interface for contract interactions.
// It provides a unified entry point for all contract operations.
type Contract interface {
	// Method creates a new method builder for the specified method and arguments.
	Method(method string, args ...any) MethodBuilder

	// FilterEvent creates a new filter builder for the specified event.
	FilterEvent(eventName string) FilterBuilder

	// Address returns the contract address.
	Address() *thor.Address

	// ABI returns the contract ABI.
	ABI() *abi.ABI

	// Client returns the underlying client.
	Client() *thorclient.Client
}

// contract is the concrete implementation of Contract.
type contract struct {
	client *thorclient.Client
	abi    *abi.ABI
	addr   *thor.Address
}

// NewContract creates a new contract instance with the given client, ABI data and address.
func NewContract(client *thorclient.Client, abiData []byte, address *thor.Address) (Contract, error) {
	if address == nil {
		return nil, fmt.Errorf("empty contract address")
	}
	contractABI, err := abi.JSON(bytes.NewReader(abiData))
	if err != nil {
		return nil, err
	}
	return &contract{
		client: client,
		abi:    &contractABI,
		addr:   address,
	}, nil
}

// DeployContract deploys a contract and creates a new contract instance with the given client, ABI data and address.
func DeployContract(client *thorclient.Client, signer Signer, abiData []byte, bytecode string) (Contract, error) {
	bc, err := hexutil.Decode(bytecode)
	if err != nil {
		return nil, err
	}

	tag, err := client.ChainTag()
	if err != nil {
		return nil, err
	}

	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(tag).
		Clause(tx.NewClause(nil).WithData(bc)).
		Expiration(10000).
		Gas(10_000_000).
		Build()
	trx, err = signer.SignTransaction(trx)
	if err != nil {
		return nil, err
	}

	signerAddr := signer.Address()
	clauses, err := client.InspectTxClauses(trx, &signerAddr)
	if err != nil {
		return nil, err
	}
	if len(clauses) != 1 || clauses[0].Reverted || clauses[0].VMError != "" {
		return nil, fmt.Errorf("unable to deploy contract: %+v", clauses)
	}

	res, err := client.SendTransaction(trx)
	if err != nil {
		return nil, err
	}

	var receipt *transactions.Receipt
	err = test.Retry(func() error {
		if receipt, err = client.TransactionReceipt(res.ID); err != nil {
			return err
		}
		return nil
	}, time.Second, 10*time.Second)
	if err != nil {
		return nil, err
	}

	if receipt.Reverted {
		return nil, fmt.Errorf("transaction reverted")
	}

	contractAddr := receipt.Outputs[0].Events[0].Address
	return NewContract(client, abiData, &contractAddr)
}

// Method implements Contract.Method.
func (c *contract) Method(method string, args ...any) MethodBuilder {
	return &methodBuilder{
		contract: c,
		method:   method,
		args:     args,
		vet:      big.NewInt(0),
	}
}

// FilterEvent implements Contract.Event.
func (c *contract) FilterEvent(eventName string) FilterBuilder {
	return &filterBuilder{
		op: &methodBuilder{
			contract: c,
			method:   eventName,
		},
	}
}

// Address returns the contract address.
func (c *contract) Address() *thor.Address {
	return c.addr
}

// ABI returns the contract ABI.
func (c *contract) ABI() *abi.ABI {
	return c.abi
}

// Client returns the underlying HTTP client.
func (c *contract) Client() *thorclient.Client {
	return c.client
}
