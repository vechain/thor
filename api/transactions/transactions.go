package transactions

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/transfers"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/eventdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type Transactions struct {
	chain      *chain.Chain
	pool       *txpool.TxPool
	transferDB *transferdb.TransferDB
}

func New(chain *chain.Chain, pool *txpool.TxPool, transferDB *transferdb.TransferDB) *Transactions {
	return &Transactions{
		chain,
		pool,
		transferDB,
	}
}

func (t *Transactions) getRawTransaction(txID thor.Bytes32) (*rawTransaction, error) {
	var transaction *tx.Transaction
	var blockC BlockContext
	if pengdingTransaction := t.pool.GetTransaction(txID); pengdingTransaction != nil {
		transaction = pengdingTransaction
	} else {
		tx, location, err := t.chain.GetTransaction(txID)
		if err != nil {
			if t.chain.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		block, err := t.chain.GetBlock(location.BlockID)
		if err != nil {
			if t.chain.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		blockC = BlockContext{
			ID:        block.Header().ID(),
			Number:    block.Header().Number(),
			Timestamp: block.Header().Timestamp(),
		}
		transaction = tx
	}
	raw, err := rlp.EncodeToBytes(transaction)
	if err != nil {
		return nil, err
	}
	return &rawTransaction{
		Block: blockC,
		RawTx: RawTx{hexutil.Encode(raw)},
	}, nil
}

func (t *Transactions) getTransactionByID(txID thor.Bytes32) (*Transaction, error) {
	if pengdingTransaction := t.pool.GetTransaction(txID); pengdingTransaction != nil {
		return ConvertTransaction(pengdingTransaction)
	}
	tx, location, err := t.chain.GetTransaction(txID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	block, err := t.chain.GetBlock(location.BlockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tc, err := ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}
	tc.Block.ID = block.Header().ID()
	tc.Block.Number = block.Header().Number()
	tc.Block.Timestamp = block.Header().Timestamp()
	return tc, nil
}

//GetTransactionReceiptByID get tx's receipt
func (t *Transactions) getTransactionReceiptByID(txID thor.Bytes32) (*Receipt, error) {
	tx, location, err := t.chain.GetTransaction(txID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	block, err := t.chain.GetBlock(location.BlockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	receipts, err := t.chain.GetBlockReceipts(block.Header().ID())
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	rece := receipts[location.Index]
	return convertReceipt(rece, block, tx)
}

func (t *Transactions) sendTx(tx *tx.Transaction) (thor.Bytes32, error) {
	if err := t.pool.Add(tx); err != nil {
		return thor.Bytes32{}, err
	}
	return tx.ID(), nil
}

func (t *Transactions) handleSendTransaction(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	var raw *RawTx
	if err = json.Unmarshal(res, &raw); err != nil {
		return err
	}
	tx, err := raw.decode()
	if err != nil {
		return err
	}
	txID, err := t.sendTx(tx)
	if err != nil {
		return utils.Forbidden(err, "rejected tx")
	}
	return utils.WriteJSON(w, map[string]string{
		"id": txID.String(),
	})
}

func (t *Transactions) handleGetTransactionByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return utils.BadRequest(err, "id")
	}
	raw := req.URL.Query().Get("raw")
	if raw != "" && raw != "false" && raw != "true" {
		return utils.BadRequest(errors.New("raw should be bool"), "raw")
	}
	if raw == "true" {
		tx, err := t.getRawTransaction(txID)
		if err != nil {
			return err
		}
		return utils.WriteJSON(w, tx)
	}
	tx, err := t.getTransactionByID(txID)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, tx)

}

func (t *Transactions) handleGetTransactionReceiptByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return utils.BadRequest(err, "id")
	}
	receipt, err := t.getTransactionReceiptByID(txID)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, receipt)
}

func (t *Transactions) getTransfers(filter *transferdb.TransferFilter) ([]*transfers.FilteredTransfer, error) {
	transferLogs, err := t.transferDB.Filter(filter)
	if err != nil {
		return nil, err
	}
	tLogs := make([]*transfers.FilteredTransfer, len(transferLogs))
	for i, trans := range transferLogs {
		tLogs[i] = transfers.ConvertTransfer(trans)
	}
	return tLogs, nil
}

func (t *Transactions) handleFilterTransferLogsByTxID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return utils.BadRequest(err, "id")
	}
	transFilter := &transferdb.TransferFilter{TxID: &txID}
	order := req.URL.Query().Get("order")
	if order != string(eventdb.DESC) {
		transFilter.Order = transferdb.ASC
	} else {
		transFilter.Order = transferdb.DESC
	}
	transferLogs, err := t.getTransfers(transFilter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, transferLogs)
}

func (t *Transactions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleSendTransaction))
	sub.Path("/{id}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))
	sub.Path("/{id}/receipt").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
	sub.Path("/{id}/transfers").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleFilterTransferLogsByTxID))
}
