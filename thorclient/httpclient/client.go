// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

type Client struct {
	url string
	c   *http.Client
}

func New(url string) *Client {
	return &Client{
		url: url,
		c:   &http.Client{},
	}
}

func (c *Client) GetTransactionReceipt(txID *thor.Bytes32, revision string) (*transactions.Receipt, error) {
	url := c.url + "/transactions/" + txID.String() + "/receipt"
	if revision != "" {
		url += "?revision=" + revision
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
		return nil, fmt.Errorf("unable to unmarshall receipt - %w", err)
	}

	return &receipt, nil
}

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
		return nil, fmt.Errorf("unable to unmarshall inspection - %w", err)
	}

	return inspectionRes, nil
}

func (c *Client) SendTransaction(obj *transactions.RawTx) (*transactions.TxSendResult, error) {
	body, err := c.httpPOST(c.url+"/transactions", obj)
	if err != nil {
		return nil, fmt.Errorf("unable to send raw transaction - %w", err)
	}

	var txID transactions.TxSendResult
	if err = json.Unmarshal(body, &txID); err != nil {
		return nil, fmt.Errorf("unable to unmarshall inspection - %w", err)
	}

	return &txID, nil
}

func (c *Client) FilterEvents(req *events.EventFilter) ([]events.FilteredEvent, error) {
	body, err := c.httpPOST(c.url+"/logs/event", req)
	if err != nil {
		return nil, fmt.Errorf("unable to send raw transaction - %w", err)
	}

	var filteredEvents []events.FilteredEvent
	if err = json.Unmarshal(body, &filteredEvents); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return filteredEvents, nil
}

func (c *Client) FilterTransfers(req *events.EventFilter) ([]*transfers.FilteredTransfer, error) {
	body, err := c.httpPOST(c.url+"/logs/transfer", req)
	if err != nil {
		return nil, fmt.Errorf("unable to send retrieve transfer logs - %w", err)
	}

	var filteredEvents []*transfers.FilteredTransfer
	if err = json.Unmarshal(body, &filteredEvents); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return filteredEvents, nil
}

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
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return &account, nil
}

func (c *Client) GetAccountCode(addr *thor.Address, revision string) (*accounts.GetCodeResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/code"
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account - %w", err)
	}

	var res accounts.GetCodeResult
	if err = json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unable to unmarshall code - %w", err)
	}

	return &res, nil
}

func (c *Client) GetAccountStorage(addr *thor.Address, key *thor.Bytes32, revision string) (*accounts.GetStorageResult, error) {
	url := c.url + "/accounts/" + addr.String() + "/key/" + key.String()
	if revision != "" {
		url += "?revision=" + revision
	}

	body, err := c.httpGET(url)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account - %w", err)
	}

	var res accounts.GetStorageResult
	if err = json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unable to unmarshall code - %w", err)
	}

	return &res, nil
}

func (c *Client) GetBlockExpanded(revision string) (*blocks.JSONExpandedBlock, error) {
	body, err := c.httpGET(c.url + "/blocks/" + revision + "?expanded=true")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve block - %w", err)
	}

	var block blocks.JSONExpandedBlock
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return &block, nil
}

func (c *Client) GetBlock(blockID string) (*blocks.JSONBlockSummary, error) {
	body, err := c.httpGET(c.url + "/blocks/" + blockID)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve block - %w", err)
	}

	var block blocks.JSONBlockSummary
	if err = json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return &block, nil
}

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
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return &tx, nil
}

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
		return nil, fmt.Errorf("unable to retrieve transaction - %w", err)
	}

	var tx transactions.RawTransaction
	if err = json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return &tx, nil
}

func (c *Client) RawHTTPPost(url string, calldata interface{}) ([]byte, int, error) {
	var data []byte
	var err error

	if _, ok := calldata.([]byte); ok {
		data = calldata.([]byte)
	} else {
		data, err = json.Marshal(calldata)
		if err != nil {
			return nil, 0, fmt.Errorf("unable to unmarshal payload - %w", err)
		}
	}

	return c.rawHTTPRequest("POST", c.url+url, bytes.NewBuffer(data))
}

func (c *Client) RawHTTPGet(url string) ([]byte, int, error) {
	return c.rawHTTPRequest("GET", c.url+url, nil)
}

func (c *Client) GetPeers() ([]*node.PeerStats, error) {
	body, err := c.httpGET(c.url + "/node/network/peers")
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve peers - %w", err)
	}

	var peers []*node.PeerStats
	if err = json.Unmarshal(body, &peers); err != nil {
		return nil, fmt.Errorf("unable to unmarshall events - %w", err)
	}

	return peers, nil
}
