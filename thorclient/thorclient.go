// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package thorclient provides a client for interacting with the VeChainThor blockchain.
// It offers a set of methods to interact with accounts, transactions, blocks, events, and other
// features via HTTP and WebSocket connections.

package thorclient

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/common"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thor/v2/thorclient/wsclient"
	"github.com/vechain/thor/v2/tx"

	tccommon "github.com/vechain/thor/v2/thorclient/common"
)

// Client represents the VeChainThor client, allowing communication over HTTP and WebSocket.
type Client struct {
	httpConn *httpclient.Client
	wsConn   *wsclient.Client
}

// New creates a new Client using the provided HTTP URL.
func New(url string) *Client {
	return &Client{
		httpConn: httpclient.New(url),
	}
}

// NewWithWS creates a new Client using the provided HTTP and WebSocket URLs.
// Returns an error if the WebSocket connection fails.
func NewWithWS(url string) (*Client, error) {
	wsClient, err := wsclient.NewClient(url)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpConn: httpclient.New(url),
		wsConn:   wsClient,
	}, nil
}

// Option represents a functional option for customizing client requests.
type Option func(*getOptions)

// getOptions holds configuration options for client requests.
type getOptions struct {
	revision string
	pending  bool
}

// applyOptions applies the given functional options to the default options.
func applyOptions(opts []Option) *getOptions {
	options := &getOptions{
		revision: tccommon.BestRevision,
		pending:  false,
	}
	for _, o := range opts {
		o(options)
	}
	return options
}

// Revision returns an Option to specify the revision for requests.
func Revision(revision string) Option {
	return func(o *getOptions) {
		o.revision = revision
	}
}

// Pending returns an Option to specify that the client should fetch pending results.
func Pending() Option {
	return func(o *getOptions) {
		o.pending = true
	}
}

// RawHTTPClient returns the underlying HTTP client.
func (c *Client) RawHTTPClient() *httpclient.Client {
	return c.httpConn
}

// RawWSClient returns the underlying WebSocket client.
func (c *Client) RawWSClient() *wsclient.Client {
	return c.wsConn
}

// Account retrieves an account from the blockchain based on the provided address and options.
func (c *Client) Account(addr *thor.Address, opts ...Option) (*accounts.Account, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccount(addr, options.revision)
}

// InspectClauses inspects the clauses of a batch call data and returns the call results.
func (c *Client) InspectClauses(calldata *accounts.BatchCallData, opts ...Option) ([]*accounts.CallResult, error) {
	options := applyOptions(opts)
	return c.httpConn.InspectClauses(calldata, options.revision)
}

// InspectTxClauses inspects the clauses of a transaction and returns the call results.
// It accepts both signed and unsigned transactions.
func (c *Client) InspectTxClauses(tx *tx.Transaction, senderAddr *thor.Address, opts ...Option) ([]*accounts.CallResult, error) {
	clauses := convertToBatchCallData(tx, senderAddr)
	return c.InspectClauses(clauses, opts...)
}

// AccountCode retrieves the account code for a given address.
func (c *Client) AccountCode(addr *thor.Address, opts ...Option) (*accounts.GetCodeResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountCode(addr, options.revision)
}

// AccountStorage retrieves the storage value for a given address and key.
func (c *Client) AccountStorage(addr *thor.Address, key *thor.Bytes32, opts ...Option) (*accounts.GetStorageResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountStorage(addr, key, options.revision)
}

// Transaction retrieves a transaction by its ID.
func (c *Client) Transaction(id *thor.Bytes32, opts ...Option) (*transactions.Transaction, error) {
	options := applyOptions(opts)
	return c.httpConn.GetTransaction(id, options.revision, options.pending)
}

// RawTransaction retrieves the raw transaction data by its ID.
func (c *Client) RawTransaction(id *thor.Bytes32, opts ...Option) (*transactions.RawTransaction, error) {
	options := applyOptions(opts)
	return c.httpConn.GetRawTransaction(id, options.revision, options.pending)
}

// TransactionReceipt retrieves the receipt for a transaction by its ID.
func (c *Client) TransactionReceipt(id *thor.Bytes32, opts ...Option) (*transactions.Receipt, error) {
	options := applyOptions(opts)
	return c.httpConn.GetTransactionReceipt(id, options.revision)
}

// SendTransaction sends a signed transaction to the blockchain.
func (c *Client) SendTransaction(tx *tx.Transaction) (*transactions.SendTxResult, error) {
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, fmt.Errorf("unable to encode transaction - %w", err)
	}

	return c.SendRawTransaction(rlpTx)
}

// SendRawTransaction sends a raw RLP-encoded transaction to the blockchain.
func (c *Client) SendRawTransaction(rlpTx []byte) (*transactions.SendTxResult, error) {
	return c.httpConn.SendTransaction(&transactions.RawTx{Raw: hexutil.Encode(rlpTx)})
}

// Block retrieves a block by its revision.
func (c *Client) Block(revision string) (blocks *blocks.JSONCollapsedBlock, err error) {
	return c.httpConn.GetBlock(revision)
}

// ExpandedBlock retrieves an expanded block by its revision.
func (c *Client) ExpandedBlock(revision string) (blocks *blocks.JSONExpandedBlock, err error) {
	return c.httpConn.GetExpandedBlock(revision)
}

// FilterEvents filters events based on the provided filter request.
func (c *Client) FilterEvents(req *events.EventFilter) ([]events.FilteredEvent, error) {
	return c.httpConn.FilterEvents(req)
}

// FilterTransfers filters transfers based on the provided filter request.
func (c *Client) FilterTransfers(req *transfers.TransferFilter) ([]*transfers.FilteredTransfer, error) {
	return c.httpConn.FilterTransfers(req)
}

// Peers retrieves the list of connected peers.
func (c *Client) Peers() ([]*node.PeerStats, error) {
	return c.httpConn.GetPeers()
}

// ChainTag retrieves the chain tag from the genesis block.
func (c *Client) ChainTag() (byte, error) {
	genesisBlock, err := c.Block("0")
	if err != nil {
		return 0, err
	}
	return genesisBlock.ID[31], nil
}

// SubscribeBlocks subscribes to block updates over WebSocket.
func (c *Client) SubscribeBlocks(pos string) (*common.Subscription[*blocks.JSONCollapsedBlock], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBlocks(pos)
}

// SubscribeEvents subscribes to event updates over WebSocket.
func (c *Client) SubscribeEvents(pos string, filter *subscriptions.EventFilter) (*common.Subscription[*subscriptions.EventMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeEvents(pos, filter)
}

// SubscribeTransfers subscribes to transfer updates over WebSocket.
func (c *Client) SubscribeTransfers(pos string, filter *subscriptions.TransferFilter) (*common.Subscription[*subscriptions.TransferMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTransfers(pos, filter)
}

// SubscribeBeats2 subscribes to Beat2 message updates over WebSocket.
func (c *Client) SubscribeBeats2(pos string) (*common.Subscription[*subscriptions.Beat2Message], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBeats2(pos)
}

// SubscribeTxPool subscribes to pending transaction updates over WebSocket.
func (c *Client) SubscribeTxPool(txID *thor.Bytes32) (*common.Subscription[*subscriptions.PendingTxIDMessage], error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTxPool(txID)
}

// convertToBatchCallData converts a transaction and sender address to batch call data format.
func convertToBatchCallData(tx *tx.Transaction, addr *thor.Address) *accounts.BatchCallData {
	cls := make(accounts.Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = convertClauseAccounts(c)
	}

	blockRef := tx.BlockRef()
	encodedBlockRef := hexutil.Encode(blockRef[:])

	return &accounts.BatchCallData{
		Clauses:    cls,
		Gas:        tx.Gas(),
		ProvedWork: nil, // todo hook this field
		Caller:     addr,
		GasPayer:   nil, // todo hook this field
		GasPrice:   nil, // todo hook this field
		Expiration: tx.Expiration(),
		BlockRef:   encodedBlockRef,
	}
}

// convertClauseAccounts converts a transaction clause to accounts.Clause format.
func convertClauseAccounts(c *tx.Clause) accounts.Clause {
	value := math.HexOrDecimal256(*c.Value())
	return accounts.Clause{
		To:    c.To(),
		Value: &value,
		Data:  hexutil.Encode(c.Data()),
	}
}
