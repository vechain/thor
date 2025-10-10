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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrNot200Status = errors.New("not 200 status code")
)

const (
	BestRevision      = "best"
	FinalizedRevision = "finalized"
)

// Client represents the HTTP client for interacting with the VeChainThor blockchain.
// It manages communication via HTTP requests.
type Client struct {
	url     string
	c       *http.Client
	genesis atomic.Pointer[api.JSONCollapsedBlock]
}

// New creates a new Client with the provided URL.
func New(url string) *Client {
	return NewWithHTTP(url, http.DefaultClient)
}

func NewWithHTTP(url string, c *http.Client) *Client {
	return &Client{
		url:     url,
		c:       c,
		genesis: atomic.Pointer[api.JSONCollapsedBlock]{},
	}
}

// GetAccount retrieves the account details for the given address at the specified revision.
func (c *Client) GetAccount(addr *thor.Address, revision string) (*api.Account, error) {
	url := c.url + "/accounts/" + addr.String()
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account - %w", err)
	}

	var account api.Account
	if err = json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("unable to unmarshal account - %w", err)
	}

	return &account, nil
}

// InspectClauses performs a clause inspection on batch call data at the specified revision.
func (c *Client) InspectClauses(calldata *api.BatchCallData, revision string) ([]*api.CallResult, error) {
	url := c.url + "/accounts/*"
	if revision != "" {
		url += "?revision=" + revision
	}
	body, err := c.httpPOST(url, calldata)
	if err != nil {
		return nil, fmt.Errorf("unable to request inspect clauses - %w", err)
	}

	var inspectionRes []*api.CallResult
	if err = json.Unmarshal(body, &inspectionRes); err != nil {
		return nil, fmt.Errorf("unable to unmarshal inspection result - %w", err)
	}

	return inspectionRes, nil
}

// GetAccountCode retrieves the contract code for the given address at the specified revision.
func (c *Client) GetAccountCode(addr *thor.Address, revision string) (*api.GetCodeResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/code"
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account code - %w", err)
	}

	var res api.GetCodeResult
	if err = json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unable to unmarshal code - %w", err)
	}

	return &res, nil
}

// GetAccountStorage retrieves the storage value for the given address and key at the specified revision.
func (c *Client) GetAccountStorage(addr *thor.Address, key *thor.Bytes32, revision string) (*api.GetStorageResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/storage/" + key.String()
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account storage - %w", err)
	}

	var res api.GetStorageResult
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
func (c *Client) GetRawTransaction(txID *thor.Bytes32, head string, isPending bool) (*api.RawTransaction, error) {
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

	var tx api.RawTransaction
	if err = json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("unable to unmarshal raw transaction - %w", err)
	}

	return &tx, nil
}

// GetTransactionReceipt retrieves the receipt for the given transaction ID at the specified head.
func (c *Client) GetTransactionReceipt(txID *thor.Bytes32, head string) (*api.Receipt, error) {
	url := c.url + "/transactions/" + txID.String() + "/receipt"
	if head != "" {
		url += "?head=" + head
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch receipt - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, ErrNotFound
	}

	var receipt api.Receipt
	if err = json.Unmarshal(body, &receipt); err != nil {
		return nil, fmt.Errorf("unable to unmarshal receipt - %w", err)
	}

	return &receipt, nil
}

// SendTransaction sends a raw transaction to the blockchain.
func (c *Client) SendTransaction(obj *api.RawTx) (*api.SendTxResult, error) {
	body, err := c.httpPOST(c.url+"/transactions", obj)
	if err != nil {
		return nil, fmt.Errorf("unable to send raw transaction - %w", err)
	}

	var txID api.SendTxResult
	if err = json.Unmarshal(body, &txID); err != nil {
		return nil, fmt.Errorf("unable to unmarshal send transaction result - %w", err)
	}

	return &txID, nil
}

// GetBlock retrieves a block by its block ID.
func (c *Client) GetBlock(blockID string) (*api.JSONCollapsedBlock, error) {
	if blockID == "0" && c.genesis.Load() != nil {
		return c.genesis.Load(), nil
	}
	body, err := c.httpGET(c.url + "/blocks/" + blockID)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve block - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, ErrNotFound
	}

	var block api.JSONCollapsedBlock
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshal block - %w", err)
	}

	if block.Number == 0 {
		// Cache the genesis block for future requests
		c.genesis.Store(&block)
	}

	return &block, nil
}

// GetExpandedBlock retrieves an expanded block by its revision.
func (c *Client) GetExpandedBlock(revision string) (*api.JSONExpandedBlock, error) {
	body, err := c.httpGET(c.url + "/blocks/" + revision + "?expanded=true")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve expanded block - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, ErrNotFound
	}

	var block api.JSONExpandedBlock
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshal expanded block - %w", err)
	}

	return &block, nil
}

// GetBlockReward retrieves a block reward and validator for block
func (c *Client) GetBlockReward(revision string) (*api.JSONBlockReward, error) {
	body, err := c.httpGET(c.url + "/blocks/reward/" + revision)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve block reward - %w", err)
	}

	if len(body) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, ErrNotFound
	}

	var blockReward api.JSONBlockReward
	if err = json.Unmarshal(body, &blockReward); err != nil {
		return nil, fmt.Errorf("unable to unmarshal block reward - %w", err)
	}

	return &blockReward, nil
}

// FilterEvents filters events based on the provided event filter.
func (c *Client) FilterEvents(req *api.EventFilter) ([]api.FilteredEvent, error) {
	body, err := c.httpPOST(c.url+"/logs/event", req)
	if err != nil {
		return nil, fmt.Errorf("unable to filter events - %w", err)
	}

	var filteredEvents []api.FilteredEvent
	if err = json.Unmarshal(body, &filteredEvents); err != nil {
		return nil, fmt.Errorf("unable to unmarshal events - %w", err)
	}

	return filteredEvents, nil
}

// FilterTransfers filters transfer based on the provided transfer filter.
func (c *Client) FilterTransfers(req *api.TransferFilter) ([]*api.FilteredTransfer, error) {
	body, err := c.httpPOST(c.url+"/logs/transfer", req)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve transfer logs - %w", err)
	}

	var filteredTransfers []*api.FilteredTransfer
	if err = json.Unmarshal(body, &filteredTransfers); err != nil {
		return nil, fmt.Errorf("unable to unmarshal transfers - %w", err)
	}

	return filteredTransfers, nil
}

// GetPeers retrieves the network peers connected to the node.
func (c *Client) GetPeers() ([]*api.PeerStats, error) {
	body, err := c.httpGET(c.url + "/node/network/peers")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve peers - %w", err)
	}

	var peers []*api.PeerStats
	if err = json.Unmarshal(body, &peers); err != nil {
		return nil, fmt.Errorf("unable to unmarshal peers - %w", err)
	}

	return peers, nil
}

// GetFeesHistory retrieves the fees history based on the block count and newest block.
func (c *Client) GetFeesHistory(blockCount uint32, newestBlock string, rewardPercentiles []float64) (*api.FeesHistory, error) {
	var url strings.Builder
	url.WriteString(c.url + "/fees/history?blockCount=" + fmt.Sprint(blockCount) + "&newestBlock=" + newestBlock)
	if len(rewardPercentiles) > 0 {
		var values []string
		for _, v := range rewardPercentiles {
			values = append(values, strconv.FormatFloat(v, 'f', -1, 64))
		}
		url.WriteString("&rewardPercentiles=" + strings.Join(values, ","))
	}
	body, err := c.httpGET(url.String())
	if err != nil {
		return nil, fmt.Errorf("unable to get the fees history - %w", err)
	}

	var history api.FeesHistory
	if err = json.Unmarshal(body, &history); err != nil {
		return nil, fmt.Errorf("unable to unmarshal the fees history - %w", err)
	}

	return &history, nil
}

// GetFeesPriority retrieves the suggested maxPriorityFeePerGas for a transaction to be included in the next blocks.
func (c *Client) GetFeesPriority() (*api.FeesPriority, error) {
	body, err := c.httpGET(c.url + "/fees/priority")
	if err != nil {
		return nil, fmt.Errorf("unable to get the fees priority - %w", err)
	}

	var priority api.FeesPriority
	if err = json.Unmarshal(body, &priority); err != nil {
		return nil, fmt.Errorf("unable to unmarshal the fees priority - %w", err)
	}

	return &priority, nil
}

// GetTxPool retrieves transactions from the transaction pool.
func (c *Client) GetTxPool(expanded bool, origin *thor.Address) (any, error) {
	url := c.url + "/node/txpool"
	params := []string{}

	if expanded {
		params = append(params, "expanded=true")
	}

	if origin != nil {
		params = append(params, "origin="+origin.String())
	}

	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to get txpool - %w", err)
	}

	if expanded {
		var transactions []transactions.Transaction
		if err = json.Unmarshal(body, &transactions); err != nil {
			return nil, fmt.Errorf("unable to unmarshal txpool transactions - %w", err)
		}
		return transactions, nil
	}

	var txIDs []thor.Bytes32
	if err = json.Unmarshal(body, &txIDs); err != nil {
		return nil, fmt.Errorf("unable to unmarshal txpool transaction IDs - %w", err)
	}
	return txIDs, nil
}

// GetTxPoolStatus retrieves the current status of the transaction pool.
func (c *Client) GetTxPoolStatus() (*api.Status, error) {
	body, err := c.httpGET(c.url + "/node/txpool/status")
	if err != nil {
		return nil, fmt.Errorf("unable to get txpool status - %w", err)
	}

	var status api.Status
	if err = json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("unable to unmarshal txpool status - %w", err)
	}

	return &status, nil
}

// RawHTTPPost sends a raw HTTP POST request to the specified URL with the provided data.
func (c *Client) RawHTTPPost(url string, calldata any) ([]byte, int, error) {
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
