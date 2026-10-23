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

	"github.com/vechain/thor/v2/abi/ethabi"
	"github.com/vechain/thor/v2/api"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

type Contract struct {
	client *thorclient.Client
	abi    *ethabi.ABI
	addr   *thor.Address
}

// NewContract creates a new contract instance with the given client, ABI data and address.
func NewContract(client *thorclient.Client, abiData []byte, address *thor.Address) (*Contract, error) {
	if address == nil {
		return nil, fmt.Errorf("empty contract address")
	}
	contractABI, err := ethabi.JSON(bytes.NewReader(abiData))
	if err != nil {
		return nil, err
	}
	return &Contract{
		client: client,
		abi:    &contractABI,
		addr:   address,
	}, nil
}

// DeployContract deploys a contract and creates a new contract instance with the given client, ABI data and address.
func DeployContract(client *thorclient.Client, signer Signer, abiData []byte, bytecode string) (*Contract, error) {
	bc, err := hexutil.Decode(bytecode)
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(nil).WithData(bc)

	// Unlike Submit, a deployment whose simulation fails is not sent.
	gas, simulation, err := estimateGas(client, signer.Address(), clause)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate deployment gas: %w", err)
	}
	if simulation.Reverted || simulation.VMError != "" {
		return nil, fmt.Errorf("unable to deploy contract: %+v", simulation)
	}

	trx, err := buildTx(client, clause, TxOptions{Gas: &gas, Expiration: new(uint32(10000))})
	if err != nil {
		return nil, err
	}
	trx, err = signer.SignTransaction(trx)
	if err != nil {
		return nil, err
	}

	res, err := client.SendTransaction(trx)
	if err != nil {
		return nil, err
	}

	var receipt *api.Receipt
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
func (c *Contract) Method(method string, args ...any) *MethodBuilder {
	return &MethodBuilder{
		contract: c,
		method:   method,
		args:     args,
		vet:      big.NewInt(0),
	}
}

// FilterEvent implements Contract.Event.
func (c *Contract) FilterEvent(eventName string) *FilterBuilder {
	return &FilterBuilder{
		op: &MethodBuilder{
			contract: c,
			method:   eventName,
		},
	}
}

// Address returns the contract address.
func (c *Contract) Address() *thor.Address {
	return c.addr
}

// ABI returns the contract ABI.
func (c *Contract) ABI() *ethabi.ABI {
	return c.abi
}

// Client returns the underlying HTTP client.
func (c *Contract) Client() *thorclient.Client {
	return c.client
}
