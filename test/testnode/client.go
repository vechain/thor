// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testnode

import (
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// Client wraps thorclient.ClientInterface to add transaction minting capabilities.
// It allows for custom minting behavior through the mintFunc field, which can be
// used to trigger block production or other minting operations before retrieving
// transaction receipts.
type Client struct {
	thorclient.ClientInterface
	mintFunc func() error // Optional function to execute before retrieving receipts
}

// TransactionReceipt retrieves the receipt for a transaction by its ID.
// If a minting function is set, it will be executed before attempting to
// retrieve the receipt. This allows for custom minting behavior such as
// triggering block production or waiting for transaction inclusion.
func (c *Client) TransactionReceipt(id *thor.Bytes32, opts ...thorclient.Option) (*transactions.Receipt, error) {
	if c.mintFunc != nil {
		if err := c.mintFunc(); err != nil {
			return nil, err
		}
	}

	return c.ClientInterface.TransactionReceipt(id, opts...)
}
