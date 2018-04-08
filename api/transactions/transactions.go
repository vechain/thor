package transactions

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
)

type Transactions struct {
	chain  *chain.Chain
	txPool *txpool.TxPool
}

func New(chain *chain.Chain, txPool *txpool.TxPool) *Transactions {
	return &Transactions{
		chain,
		txPool,
	}
}

func (t *Transactions) getTransactionByID(txID thor.Bytes32) (*Transaction, error) {
	if pengdingTransaction := t.txPool.GetTransaction(txID); pengdingTransaction != nil {
		return ConvertTransaction(pengdingTransaction)
	}
	tx, location, err := t.chain.GetTransaction(txID)
	if err != nil {
		return nil, err
	}
	tj, err := ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}
	block, err := t.chain.GetBlock(location.BlockID)
	if err != nil {
		return nil, err
	}
	tj.BlockID = location.BlockID
	tj.BlockNumber = block.Header().Number()
	tj.TxIndex = location.Index
	return tj, nil
}

//GetTransactionReceiptByID get tx's receipt
func (t *Transactions) getTransactionReceiptByID(txID thor.Bytes32) (*Receipt, error) {
	rece, err := t.chain.GetTransactionReceipt(txID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	receipt := convertReceipt(rece)
	return receipt, nil
}

//SendRawTransaction send a raw transactoion
func (t *Transactions) sendRawTransaction(raw *RawTransaction) (*thor.Bytes32, error) {
	tx, err := buildRawTransaction(raw)
	if err != nil {
		return nil, err
	}
	if err := t.txPool.Add(tx); err != nil {
		return nil, err
	}
	txID := tx.ID()
	return &txID, nil
}

func (t *Transactions) handleSendTransaction(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	req.Body.Close()
	var rt *RawTransaction
	if err := json.Unmarshal(res, &rt); err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	txID, err := t.sendRawTransaction(rt)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, txID.String())
}

func (t *Transactions) handleGetTransactionByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "id"), http.StatusBadRequest)
	}
	tx, err := t.getTransactionByID(txID)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, tx)
}

func (t *Transactions) handleGetTransactionReceiptByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "id"), http.StatusBadRequest)
	}
	receipt, err := t.getTransactionReceiptByID(txID)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, receipt)
}

func (t *Transactions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleSendTransaction))
	sub.Path("/{id}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))
	sub.Path("/{id}/receipts").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
}
