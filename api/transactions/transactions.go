package transactions

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type Transactions struct {
	chain *chain.Chain
	pool  *txpool.TxPool
}

func New(chain *chain.Chain, pool *txpool.TxPool) *Transactions {
	return &Transactions{
		chain,
		pool,
	}
}

func (t *Transactions) getRawTransaction(txID thor.Bytes32) (*rawTransaction, error) {
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
	raw, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, err
	}
	return &rawTransaction{
		Block: BlockContext{
			ID:        block.Header().ID(),
			Number:    block.Header().Number(),
			Timestamp: block.Header().Timestamp(),
		},
		RawTx: RawTx{hexutil.Encode(raw)},
	}, nil
}

func (t *Transactions) getTransactionByID(txID thor.Bytes32) (*Transaction, error) {
	tx, location, err := t.chain.GetTransaction(txID)
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
	block, err := t.chain.GetBlock(location.BlockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tc.Block = BlockContext{
		ID:        block.Header().ID(),
		Number:    block.Header().Number(),
		Timestamp: block.Header().Timestamp(),
	}
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
	var raw *RawTx
	if err := utils.ParseJSON(req.Body, &raw); err != nil {
		return err
	}
	req.Body.Close()
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

func (t *Transactions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleSendTransaction))
	sub.Path("/{id}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))
	sub.Path("/{id}/receipt").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
}
