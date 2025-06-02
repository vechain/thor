// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"bytes"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/httpclient"
)

// Contract is the main interface for contract interactions.
// It provides a unified entry point for all contract operations.
type Contract interface {
	// Operation creates a new operation builder for the specified method and arguments.
	Operation(method string, args ...any) OperationBuilder

	// FilterEvent creates a new filter builder for the specified event.
	FilterEvent(eventName string) FilterBuilder

	// Address returns the contract address.
	Address() *thor.Address

	// ABI returns the contract ABI.
	ABI() *abi.ABI

	// Client returns the underlying HTTP client.
	Client() *httpclient.Client
}

// contract is the concrete implementation of Contract.
type contract struct {
	client *httpclient.Client
	abi    *abi.ABI
	addr   *thor.Address
}

// NewContract creates a new contract instance.
func NewContract(client *httpclient.Client, abiData []byte, address *thor.Address) (Contract, error) {
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

// Operation implements Contract.Operation.
func (c *contract) Operation(method string, args ...any) OperationBuilder {
	return newOperationBuilder(c, method, args...)
}

// FilterEvent implements Contract.Event.
func (c *contract) FilterEvent(eventName string) FilterBuilder {
	return newFilterBuilder(&operationBuilder{
		contract: c,
		method:   eventName,
	})
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
func (c *contract) Client() *httpclient.Client {
	return c.client
}
