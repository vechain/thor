// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"

	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thor/v2/thorclient/wsclient"
)

// NodeClient is a client that implements the thorclient.ClientInterface
// with additional minting functionality for transaction receipts.
type NodeClient struct {
	thorclient.ClientInterface
	httpConn *httpclient.Client
	wsConn   *wsclient.Client
	node     *Node
}

// NewNodeClient creates a new NodeClient that wraps a thorclient.ClientInterface
// and adds minting functionality.
func NewNodeClient(client thorclient.ClientInterface, node *Node) *NodeClient {
	return &NodeClient{
		ClientInterface: client,
		httpConn:        client.RawHTTPClient(),
		wsConn:          client.RawWSClient(),
		node:            node,
	}
}

// mintReceipt attempts to mint a receipt for a transaction if needed.
func (c *NodeClient) mintReceipt(txID *thor.Bytes32) error {
	// TODO: Implement actual minting logic using the node's capabilities
	// This could involve:
	// 1. Checking if the transaction is in the pool
	// 2. Triggering block production if needed
	// 3. Waiting for the transaction to be included in a block
	// 4. Any other minting-specific operations
	return nil
}

// TransactionReceipt retrieves the receipt for a transaction by its ID.
func (c *NodeClient) TransactionReceipt(id *thor.Bytes32, opts ...thorclient.Option) (*transactions.Receipt, error) {
	fmt.Printf("Getting receipt for transaction: %s\n", id.String())
	return c.ClientInterface.TransactionReceipt(id, opts...)
}

// Ensure NodeClient implements thorclient.ClientInterface
var _ thorclient.ClientInterface = (*NodeClient)(nil)
