// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

type Client struct {
	httpConn *httpclient.Client
	wsConn   *wsclient.Client
}

func New(url string) *Client {
	return &Client{
		httpConn: httpclient.New(url),
	}
}

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

type Option func(*getOptions)

type getOptions struct {
	revision string
	pending  bool
}

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

func Revision(revision string) Option {
	return func(o *getOptions) {
		o.revision = revision
	}
}

func Pending() Option {
	return func(o *getOptions) {
		o.pending = true
	}
}

func (c *Client) RawHTTPClient() *httpclient.Client {
	return c.httpConn
}
func (c *Client) RawWSClient() *wsclient.Client {
	return c.wsConn
}

func (c *Client) TransactionReceipt(id *thor.Bytes32, opts ...Option) (*transactions.Receipt, error) {
	options := applyOptions(opts)
	return c.httpConn.GetTransactionReceipt(id, options.revision)
}

func (c *Client) InspectClauses(calldata *accounts.BatchCallData, opts ...Option) ([]*accounts.CallResult, error) {
	options := applyOptions(opts)
	return c.httpConn.InspectClauses(calldata, options.revision)
}

func (c *Client) InspectTxClauses(tx *tx.Transaction, senderAddr *thor.Address, opts ...Option) ([]*accounts.CallResult, error) {
	clauses := convertToBatchCallData(tx, senderAddr)
	return c.InspectClauses(clauses, opts...)
}

func (c *Client) SendTransaction(tx *tx.Transaction) (*transactions.SendTxResult, error) {
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, fmt.Errorf("unable to encode transaction - %w", err)
	}

	return c.SendTransactionRaw(rlpTx)
}

func (c *Client) SendTransactionRaw(rlpTx []byte) (*transactions.SendTxResult, error) {
	return c.httpConn.SendTransaction(&transactions.RawTx{Raw: hexutil.Encode(rlpTx)})
}

func (c *Client) FilterEvents(req *events.EventFilter) ([]events.FilteredEvent, error) {
	return c.httpConn.FilterEvents(req)
}

func (c *Client) FilterTransfers(req *events.EventFilter) ([]*transfers.FilteredTransfer, error) {
	return c.httpConn.FilterTransfers(req)
}

func (c *Client) Account(addr *thor.Address, opts ...Option) (*accounts.Account, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccount(addr, options.revision)
}

func (c *Client) AccountCode(addr *thor.Address, opts ...Option) (*accounts.GetCodeResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountCode(addr, options.revision)
}

func (c *Client) Storage(addr *thor.Address, key *thor.Bytes32, opts ...Option) (*accounts.GetStorageResult, error) {
	options := applyOptions(opts)
	return c.httpConn.GetAccountStorage(addr, key, options.revision)
}

func (c *Client) ExpandedBlock(revision string) (blocks *blocks.JSONExpandedBlock, err error) {
	return c.httpConn.GetBlockExpanded(revision)
}

func (c *Client) Block(revision string) (blocks *blocks.JSONCollapsedBlock, err error) {
	return c.httpConn.GetBlock(revision)
}

func (c *Client) Transaction(id *thor.Bytes32, opts ...Option) (*transactions.Transaction, error) {
	options := applyOptions(opts)
	return c.httpConn.GetTransaction(id, options.revision, options.pending)
}

func (c *Client) RawTransaction(id *thor.Bytes32, opts ...Option) (*transactions.RawTransaction, error) {
	options := applyOptions(opts)
	return c.httpConn.GetRawTransaction(id, options.revision, options.pending)
}

func (c *Client) Peers() ([]*node.PeerStats, error) {
	return c.httpConn.GetPeers()
}

func (c *Client) ChainTag() (byte, error) {
	genesisBlock, err := c.Block("0")
	if err != nil {
		return 0, err
	}
	return genesisBlock.ID[31], nil
}

func (c *Client) SubscribeBlocks() (blocks <-chan common.EventWrapper[*blocks.JSONCollapsedBlock], err error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBlocks("")
}

func (c *Client) SubscribeEvents() (blocks <-chan common.EventWrapper[*subscriptions.EventMessage], err error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeEvents("")
}

func (c *Client) SubscribeTransfers() (blocks <-chan common.EventWrapper[*subscriptions.TransferMessage], err error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTransfers("")
}

func (c *Client) SubscribeTxPool() (blocks <-chan common.EventWrapper[*subscriptions.PendingTxIDMessage], err error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeTxPool("")
}

func (c *Client) SubscribeBeats2() (blocks <-chan common.EventWrapper[*subscriptions.Beat2Message], err error) {
	if c.wsConn == nil {
		return nil, fmt.Errorf("not a websocket typed client")
	}
	return c.wsConn.SubscribeBeats2("")
}

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

func convertClauseAccounts(c *tx.Clause) accounts.Clause {
	value := math.HexOrDecimal256(*c.Value())
	return accounts.Clause{
		To:    c.To(),
		Value: &value,
		Data:  hexutil.Encode(c.Data()),
	}
}
