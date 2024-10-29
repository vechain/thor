// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package httpclient provides an HTTP client to interact with the VeChainThor blockchain.
// It offers various methods to retrieve accounts, transactions, blocks, events, and other blockchain data
// through HTTP requests.
package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/common"
)

// Client represents the HTTP client for interacting with the VeChainThor blockchain.
// It manages communication via HTTP requests.
type Client struct {
	url string
	c   *http.Client
}

// New creates a new Client with the provided URL.
func New(url string) *Client {
	return &Client{
		url: url,
		c:   &http.Client{},
	}
}

// GetAccount retrieves the account details for the given address at the specified revision.
func (c *Client) GetAccount(addr *thor.Address, revision string) (*accounts.Account, error) {
	url := c.url + "/accounts/" + addr.String()
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account - %w", err)
	}

	var account accounts.Account
	if err = json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("unable to unmarshal account - %w", err)
	}

	return &account, nil
}

// InspectClauses performs a clause inspection on batch call data at the specified revision.
func (c *Client) InspectClauses(calldata *accounts.BatchCallData, revision string) ([]*accounts.CallResult, error) {
	url := c.url + "/accounts/*"
	if revision != "" {
		url += "?revision=" + revision
	}
	body, err := c.httpPOST(url, calldata)
	if err != nil {
		return nil, fmt.Errorf("unable to request inspect clauses - %w", err)
	}

	var inspectionRes []*accounts.CallResult
	if err = json.Unmarshal(body, &inspectionRes); err != nil {
		return nil, fmt.Errorf("unable to unmarshal inspection result - %w", err)
	}

	return inspectionRes, nil
}

// GetAccountCode retrieves the contract code for the given address at the specified revision.
func (c *Client) GetAccountCode(addr *thor.Address, revision string) (*accounts.GetCodeResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/code"
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account code - %w", err)
	}

	var res accounts.GetCodeResult
	if err = json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unable to unmarshal code - %w", err)
	}

	return &res, nil
}

// GetAccountStorage retrieves the storage value for the given address and key at the specified revision.
func (c *Client) GetAccountStorage(addr *thor.Address, key *thor.Bytes32, revision string) (*accounts.GetStorageResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/key/" + key.String()
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account storage - %w", err)
	}

	var res accounts.GetStorageResult
	if err = json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unable to unmarshal storage result - %w", err)
	}

	return &res, nil
}

// GetTransaction retrieves the transaction details by the transaction ID, along with options for head and pending status.
func (c *Client) GetTransaction(txID *thor.Bytes32, head string, isPending bool) (*transactions.Transaction, error) {
	url := c.url + "/transactions/" + txID.String() + "?"
	if isPending {
		url += "pending=true&"
	}
	if head != "" {
		url += "head=" + head
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve transaction - %w", err)
	}

	var tx transactions.Transaction
	if err = json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("unable to unmarshal transaction - %w", err)
	}

	return &tx, nil
}

// GetRawTransaction retrieves the raw transaction data by the transaction ID, along with options for head and pending status.
func (c *Client) GetRawTransaction(txID *thor.Bytes32, head string, isPending bool) (*transactions.RawTransaction, error) {
	url := c.url + "/transactions/" + txID.String() + "?raw=true&"
	if isPending {
		url += "pending=true&"
	}
	if head != "" {
		url += "head=" + head
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve raw transaction - %w", err)
	}

	var tx transactions.RawTransaction
	if err = json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("unable to unmarshal raw transaction - %w", err)
	}

	return &tx, nil
}

// GetTransactionReceipt retrieves the receipt for the given transaction ID at the specified head.
func (c *Client) GetTransactionReceipt(txID *thor.Bytes32, head string) (*transactions.Receipt, error) {
	url := c.url + "/transactions/" + txID.String() + "/receipt"
	if head != "" {
		url += "?head=" + head
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch receipt - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, common.ErrNotFound
	}

	var receipt transactions.Receipt
	if err = json.Unmarshal(body, &receipt); err != nil {
		return nil, fmt.Errorf("unable to unmarshal receipt - %w", err)
	}

	return &receipt, nil
}

// SendTransaction sends a raw transaction to the blockchain.
func (c *Client) SendTransaction(obj *transactions.RawTx) (*transactions.SendTxResult, error) {
	body, err := c.httpPOST(c.url+"/transactions", obj)
	if err != nil {
		return nil, fmt.Errorf("unable to send raw transaction - %w", err)
	}

	var txID transactions.SendTxResult
	if err = json.Unmarshal(body, &txID); err != nil {
		return nil, fmt.Errorf("unable to unmarshal send transaction result - %w", err)
	}

	return &txID, nil
}

// GetBlock retrieves a block by its block ID.
func (c *Client) GetBlock(blockID string) (*blocks.JSONCollapsedBlock, error) {
	body, err := c.httpGET(c.url + "/blocks/" + blockID)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve block - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, common.ErrNotFound
	}

	var block blocks.JSONCollapsedBlock
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshal block - %w", err)
	}

	return &block, nil
}

// GetExpandedBlock retrieves an expanded block by its revision.
func (c *Client) GetExpandedBlock(revision string) (*blocks.JSONExpandedBlock, error) {
	body, err := c.httpGET(c.url + "/blocks/" + revision + "?expanded=true")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve expanded block - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, common.ErrNotFound
	}

	var block blocks.JSONExpandedBlock
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshal expanded block - %w", err)
	}

	return &block, nil
}

// FilterEvents filters events based on the provided event filter.
func (c *Client) FilterEvents(req *events.EventFilter) ([]events.FilteredEvent, error) {
	body, err := c.httpPOST(c.url+"/logs/event", req)
	if err != nil {
		return nil, fmt.Errorf("unable to filter events - %w", err)
	}

	var filteredEvents []events.FilteredEvent
	if err = json.Unmarshal(body, &filteredEvents); err != nil {
		return nil, fmt.Errorf("unable to unmarshal events - %w", err)
	}

	return filteredEvents, nil
}

// FilterTransfers filters transfer based on the provided transfer filter.
func (c *Client) FilterTransfers(req *transfers.TransferFilter) ([]*transfers.FilteredTransfer, error) {
	body, err := c.httpPOST(c.url+"/logs/transfer", req)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve transfer logs - %w", err)
	}

	var filteredTransfers []*transfers.FilteredTransfer
	if err = json.Unmarshal(body, &filteredTransfers); err != nil {
		return nil, fmt.Errorf("unable to unmarshal transfers - %w", err)
	}

	return filteredTransfers, nil
}

// GetPeers retrieves the network peers connected to the node.
func (c *Client) GetPeers() ([]*node.PeerStats, error) {
	body, err := c.httpGET(c.url + "/node/network/peers")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve peers - %w", err)
	}

	var peers []*node.PeerStats
	if err = json.Unmarshal(body, &peers); err != nil {
		return nil, fmt.Errorf("unable to unmarshal peers - %w", err)
	}

	return peers, nil
}

// RawHTTPPost sends a raw HTTP POST request to the specified URL with the provided data.
func (c *Client) RawHTTPPost(url string, calldata interface{}) ([]byte, int, error) {
	var data []byte
	var err error

	if _, ok := calldata.([]byte); ok {
		data = calldata.([]byte)
	} else {
		data, err = json.Marshal(calldata)
		if err != nil {
			return nil, 0, fmt.Errorf("unable to marshal payload - %w", err)
		}
	}

	return c.rawHTTPRequest("POST", c.url+url, bytes.NewBuffer(data))
}

// RawHTTPGet sends a raw HTTP GET request to the specified URL.
func (c *Client) RawHTTPGet(url string) ([]byte, int, error) {
	return c.rawHTTPRequest("GET", c.url+url, nil)
}
