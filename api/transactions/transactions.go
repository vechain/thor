// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

type Transactions struct {
	repo *chain.Repository
	pool txpool.Pool
}

func New(repo *chain.Repository, pool txpool.Pool) *Transactions {
	return &Transactions{
		repo,
		pool,
	}
}

func (t *Transactions) getRawTransaction(txID thor.Bytes32, head thor.Bytes32, allowPending bool) (*api.RawTransaction, error) {
	chain := t.repo.NewChain(head)
	tx, meta, err := chain.GetTransaction(txID)
	if err != nil {
		if t.repo.IsNotFound(err) {
			if allowPending {
				if pending := t.pool.Get(txID); pending != nil {
					raw, err := pending.MarshalBinary()
					if err != nil {
						return nil, err
					}
					return &api.RawTransaction{
						RawTx: api.RawTx{Raw: hexutil.Encode(raw)},
					}, nil
				}
			}
			return nil, nil
		}
		return nil, err
	}

	header, err := chain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, err
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &api.RawTransaction{
		RawTx: api.RawTx{Raw: hexutil.Encode(raw)},
		Meta: &api.TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		},
	}, nil
}

func (t *Transactions) getTransactionByID(txID thor.Bytes32, head thor.Bytes32, allowPending bool) (*Transaction, error) {
	chain := t.repo.NewChain(head)
	tx, meta, err := chain.GetTransaction(txID)
	if err != nil {
		if t.repo.IsNotFound(err) {
			if allowPending {
				if pending := t.pool.Get(txID); pending != nil {
					return ConvertTransaction(pending, nil), nil
				}
			}
			return nil, nil
		}
		return nil, err
	}

	header, err := chain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, err
	}
	return ConvertTransaction(tx, header), nil
}

// GetTransactionReceiptByID get tx's receipt
func (t *Transactions) getTransactionReceiptByID(txID thor.Bytes32, head thor.Bytes32) (*api.Receipt, error) {
	chain := t.repo.NewChain(head)
	tx, meta, err := chain.GetTransaction(txID)
	if err != nil {
		if t.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	receipt, err := chain.GetTransactionReceipt(txID)
	if err != nil {
		return nil, err
	}

	header, err := chain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, err
	}

	return api.ConvertReceipt(receipt, header, tx)
}

func (t *Transactions) handleSendTransaction(w http.ResponseWriter, req *http.Request) error {
	var rawTx *api.RawTx
	if err := restutil.ParseJSON(req.Body, &rawTx); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	if rawTx == nil {
		return restutil.BadRequest(errors.New("body"))
	}
	tx, err := rawTx.Decode()
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "raw"))
	}

	if err := t.pool.AddLocal(tx); err != nil {
		if txpool.IsBadTx(err) {
			return restutil.BadRequest(err)
		}
		if txpool.IsTxRejected(err) {
			return restutil.Forbidden(err)
		}
		return err
	}
	txID := tx.ID()
	return restutil.WriteJSON(w, &api.SendTxResult{ID: &txID})
}

func (t *Transactions) handleGetTransactionByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "id"))
	}

	head, err := t.parseHead(req.URL.Query().Get("head"))
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "head"))
	}
	if _, err := t.repo.GetBlockSummary(head); err != nil {
		if t.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "head"))
		}
	}

	raw := req.URL.Query().Get("raw")
	if raw != "" && raw != "false" && raw != "true" {
		return restutil.BadRequest(errors.WithMessage(errors.New("should be boolean"), "raw"))
	}
	pending := req.URL.Query().Get("pending")
	if pending != "" && pending != "false" && pending != "true" {
		return restutil.BadRequest(errors.WithMessage(errors.New("should be boolean"), "pending"))
	}

	if raw == "true" {
		tx, err := t.getRawTransaction(txID, head, pending == "true")
		if err != nil {
			return err
		}
		return restutil.WriteJSON(w, tx)
	}
	tx, err := t.getTransactionByID(txID, head, pending == "true")
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, tx)
}

func (t *Transactions) handleGetTransactionReceiptByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	txID, err := thor.ParseBytes32(id)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "id"))
	}

	head, err := t.parseHead(req.URL.Query().Get("head"))
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "head"))
	}

	if _, err := t.repo.GetBlockSummary(head); err != nil {
		if t.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "head"))
		}
	}

	receipt, err := t.getTransactionReceiptByID(txID, head)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, receipt)
}

func (t *Transactions) parseHead(head string) (thor.Bytes32, error) {
	if head == "" {
		return t.repo.BestBlockSummary().Header.ID(), nil
	}
	h, err := thor.ParseBytes32(head)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return h, nil
}

func (t *Transactions) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /transactions").
		HandlerFunc(restutil.WrapHandlerFunc(t.handleSendTransaction))
	sub.Path("/{id}").
		Methods(http.MethodGet).
		Name("GET /transactions/{id}").
		HandlerFunc(restutil.WrapHandlerFunc(t.handleGetTransactionByID))
	sub.Path("/{id}/receipt").
		Methods(http.MethodGet).
		Name("GET /transactions/{id}/receipt").
		HandlerFunc(restutil.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
}
