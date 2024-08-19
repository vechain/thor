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
)

type Client struct {
	httpConn *httpclient.Client
	wsConn   *wsclient.Client
}

func New(url string) *Client {
	return &Client{
		httpConn: httpclient.NewClient(url),
	}
}

func NewWithWS(url string) (*Client, error) {
	wsClient, err := wsclient.NewClient(url)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpConn: httpclient.NewClient(url),
		wsConn:   wsClient,
	}, nil
}

func (c *Client) RawHTTPClient() *httpclient.Client {
	return c.httpConn
}
func (c *Client) RawWSClient() *wsclient.Client {
	return c.wsConn
}

func (c *Client) GetTransactionReceipt(id *thor.Bytes32) (*transactions.Receipt, error) {
	return c.httpConn.GetTransactionReceipt(id)
}

func (c *Client) InspectClauses(calldata *accounts.BatchCallData) ([]*accounts.CallResult, error) {
	return c.httpConn.InspectClauses(calldata)
}

func (c *Client) InspectTxClauses(tx *tx.Transaction, senderAddr *thor.Address) ([]*accounts.CallResult, error) {
	clauses := convertToBatchCallData(tx, senderAddr)
	return c.InspectClauses(clauses)
}

func (c *Client) SendTransaction(tx *tx.Transaction) (*common.TxSendResult, error) {
	rlpTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, fmt.Errorf("unable to encode transaction - %w", err)
	}

	return c.SendTransactionRaw(rlpTx)
}

func (c *Client) SendTransactionRaw(rlpTx []byte) (*common.TxSendResult, error) {
	return c.httpConn.SendTransaction(&transactions.RawTx{Raw: hexutil.Encode(rlpTx)})
}

func (c *Client) FilterEvents(req *events.EventFilter) ([]events.FilteredEvent, error) {
	return c.httpConn.FilterEvents(req)
}

func (c *Client) FilterTransfers(req *events.EventFilter) ([]*transfers.FilteredTransfer, error) {
	return c.httpConn.FilterTransfers(req)
}

func (c *Client) GetAccount(addr *thor.Address) (*accounts.Account, error) {
	return c.httpConn.GetAccount(addr, "")
}

func (c *Client) GetAccountForRevision(addr *thor.Address, revision string) (*accounts.Account, error) {
	return c.httpConn.GetAccount(addr, revision)
}

func (c *Client) GetAccountCode(addr *thor.Address) ([]byte, error) {
	return c.httpConn.GetAccountCode(addr, "")
}

func (c *Client) GetAccountCodeForRevision(addr *thor.Address, revision string) ([]byte, error) {
	return c.httpConn.GetAccountCode(addr, revision)
}

func (c *Client) GetStorage(addr *thor.Address, key *thor.Bytes32) ([]byte, error) {
	return c.httpConn.GetStorage(addr, key)
}

func (c *Client) GetBlockExpanded(block string) (blocks *blocks.JSONExpandedBlock, err error) {
	return c.httpConn.GetBlockExpanded(block)
}

func (c *Client) GetBlock(block string) (blocks *blocks.JSONBlockSummary, err error) {
	return c.httpConn.GetBlock(block)
}

func (c *Client) GetTransaction(id *thor.Bytes32) (*transactions.Transaction, error) {
	return c.httpConn.GetTransaction(id, false)
}

func (c *Client) GetTransactionPending(id thor.Bytes32) (*transactions.Transaction, error) {
	return c.httpConn.GetTransaction(&id, true)
}

func (c *Client) GetPeers() ([]*node.PeerStats, error) {
	return c.httpConn.GetPeers()
}

func (c *Client) ChainTag() (byte, error) {
	genesisBlock, err := c.GetBlock("0")
	if err != nil {
		return 0, err
	}
	return genesisBlock.ID[31], nil
}

func (c *Client) SubscribeBlocks() (blocks <-chan common.EventWrapper[*blocks.JSONBlockSummary], err error) {
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
