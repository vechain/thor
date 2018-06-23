// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
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

func (t *Transactions) getRawTransaction(txID thor.Bytes32, blockID thor.Bytes32) (*rawTransaction, error) {
	txMeta, err := t.chain.GetTransactionMeta(txID, blockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tx, err := t.chain.GetTransaction(txMeta.BlockID, txMeta.Index)
	if err != nil {
		return nil, err
	}
	block, err := t.chain.GetBlock(txMeta.BlockID)
	if err != nil {
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

func (t *Transactions) getTransactionByID(txID thor.Bytes32, blockID thor.Bytes32) (*Transaction, error) {
	txMeta, err := t.chain.GetTransactionMeta(txID, blockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tx, err := t.chain.GetTransaction(txMeta.BlockID, txMeta.Index)
	if err != nil {
		return nil, err
	}
	tc, err := ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}
	h, err := t.chain.GetBlockHeader(txMeta.BlockID)
	if err != nil {
		return nil, err
	}
	tc.Block = BlockContext{
		ID:        h.ID(),
		Number:    h.Number(),
		Timestamp: h.Timestamp(),
	}
	return tc, nil
}

//GetTransactionReceiptByID get tx's receipt
func (t *Transactions) getTransactionReceiptByID(txID thor.Bytes32, blockID thor.Bytes32) (*Receipt, error) {
	txMeta, err := t.chain.GetTransactionMeta(txID, blockID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	tx, err := t.chain.GetTransaction(txMeta.BlockID, txMeta.Index)
	if err != nil {
		return nil, err
	}
	h, err := t.chain.GetBlockHeader(txMeta.BlockID)
	if err != nil {
		return nil, err
	}
	receipt, err := t.chain.GetTransactionReceipt(txMeta.BlockID, txMeta.Index)
	if err != nil {
		return nil, err
	}
	return convertReceipt(receipt, h, tx)
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
		return utils.BadRequest(err, "body")
	}
	tx, err := raw.decode()
	if err != nil {
		return utils.BadRequest(err, "raw")
	}

	txID, err := t.sendTx(tx)
	if err != nil {
		if txpool.IsBadTx(err) {
			return utils.BadRequest(err, "bad tx")
		}
		if txpool.IsTxRejected(err) {
			return utils.Forbidden(err, "rejected tx")
		}
		return err
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
	h, err := t.getBlockHeader(req.URL.Query().Get("head"))
	if err != nil {
		return err
	} else if h == nil {
		return utils.WriteJSON(w, nil)
	}
	raw := req.URL.Query().Get("raw")
	if raw != "" && raw != "false" && raw != "true" {
		return utils.BadRequest(errors.New("should be boolean"), "raw")
	}
	if raw == "true" {
		tx, err := t.getRawTransaction(txID, h.ID())
		if err != nil {
			return err
		}
		return utils.WriteJSON(w, tx)
	}
	tx, err := t.getTransactionByID(txID, h.ID())
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
	h, err := t.getBlockHeader(req.URL.Query().Get("head"))
	if err != nil {
		return err
	} else if h == nil {
		return utils.WriteJSON(w, nil)
	}
	receipt, err := t.getTransactionReceiptByID(txID, h.ID())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, receipt)
}

func (t *Transactions) getBlockHeader(head string) (*block.Header, error) {
	if head == "" {
		return t.chain.BestBlock().Header(), nil
	}
	blkID, err := thor.ParseBytes32(head)
	if err != nil {
		return nil, utils.BadRequest(err, "head")
	}
	b, err := t.chain.GetBlock(blkID)
	if err != nil {
		if t.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return b.Header(), nil
}

func (t *Transactions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleSendTransaction))

	sub.Path("/{id}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))
	sub.Path("/{id}").Methods("GET").Queries("head", "{head}").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))

	sub.Path("/{id}/receipt").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
	sub.Path("/{id}/receipt").Methods("GET").Queries("head", "{head}").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
}
