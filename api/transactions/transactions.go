// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

type Transactions struct {
	chain        *chain.Chain
	stateCreator *state.Creator
	pool         *txpool.TxPool
}

func New(chain *chain.Chain, pool *txpool.TxPool, stateCreator *state.Creator) *Transactions {
	return &Transactions{
		chain,
		stateCreator,
		pool,
	}
}

func (t *Transactions) traceTransaction(ctx context.Context, txID thor.Bytes32, blockID thor.Bytes32, transactionTrace *TransactionTrace) (interface{}, error) {
	tx, err := t.getTransactionByID(txID, blockID)
	if err != nil {
		return nil, err
	} else if tx == nil {
		return nil, nil
	}
	if transactionTrace.ClauseIndex >= uint32(len(tx.Clauses)) {
		return nil, fmt.Errorf("clase index out of range for tx %v", txID)
	}
	st, h, tctx, lg, err := t.computeTxEnv(tx, transactionTrace.ClauseIndex)
	if err != nil {
		return nil, err
	}
	if st.Err() != nil {
		return nil, st.Err()
	}
	return t.traceTx(ctx, tx, tctx, lg, h, st, transactionTrace)
}

func (t *Transactions) computeTxEnv(tx *Transaction, clauseIndex uint32) (*state.State, *block.Header, *xenv.TransactionContext, uint64, error) {
	parantBlk, err := t.chain.GetTrunkBlock(tx.Block.Number - 1)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	st, err := t.stateCreator.NewState(parantBlk.Header().StateRoot())
	if err != nil {
		return nil, nil, nil, 0, err
	}
	blk, err := t.chain.GetTrunkBlock(tx.Block.Number)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	header := blk.Header()
	signer, _ := header.Signer()
	rt := runtime.New(t.chain.NewSeeker(parantBlk.Header().ID()), st,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore()})
	for i, ltx := range blk.Transactions() {
		if ltx.ID() == tx.ID {
			tx, err := t.chain.GetTransaction(blk.Header().ID(), uint64(i))
			if err != nil {
				return nil, nil, nil, 0, err
			}
			resolvedTx, err := runtime.ResolveTransaction(tx)
			if err != nil {
				return nil, nil, nil, 0, err
			}
			baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
			gasPrice := tx.GasPrice(baseGasPrice)
			txCtx := resolvedTx.ToContext(gasPrice, rt.Context().Number, rt.Seeker().GetID)
			leftOverGas := tx.Gas() - resolvedTx.IntrinsicGas
			checkpoint := st.NewCheckpoint()
			for index, clause := range tx.Clauses() {
				if uint32(index) == clauseIndex {
					return st, header, txCtx, leftOverGas, nil
				}
				output := rt.ExecuteClause(clause, uint32(index), leftOverGas, txCtx)
				gasUsed := leftOverGas - output.LeftOverGas
				leftOverGas = output.LeftOverGas
				refund := gasUsed / 2
				if refund > output.RefundGas {
					refund = output.RefundGas
				}
				leftOverGas += refund
				if output.VMErr != nil {
					st.RevertTo(checkpoint)
					return nil, nil, nil, 0, fmt.Errorf("clause %v reverted: %v", index, output.VMErr)
				}
			}
		}
		if _, err := rt.ExecuteTransaction(ltx); err != nil {
			return nil, nil, nil, 0, err
		}
	}
	return nil, nil, nil, 0, fmt.Errorf("tx %v index out of range for block %v", tx.ID, tx.Block.ID)

}

func (t *Transactions) traceTx(ctx context.Context, txm *Transaction, txCtx *xenv.TransactionContext, leftOverGas uint64, header *block.Header, state *state.State, transactionTrace *TransactionTrace) (interface{}, error) {
	var (
		tracer vm.Tracer
		err    error
	)
	config := transactionTrace.TraceConfig
	switch {
	case config != nil && config.Tracer != nil:
		timeout := 5 * time.Second
		if config.Timeout != nil {
			if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
				return nil, err
			}
		}
		if tracer, err = tracers.New(*config.Tracer); err != nil {
			return nil, err
		}
		deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
		go func() {
			<-deadlineCtx.Done()
			tracer.(*tracers.Tracer).Stop(errors.New("execution timeout"))
		}()
		defer cancel()

	case config == nil:
		tracer = vm.NewStructLogger(nil)

	default:
		tracer = vm.NewStructLogger(convertLogConfig(transactionTrace.LogConfig))
	}

	signer, _ := header.Signer()
	rt := runtime.New(t.chain.NewSeeker(header.ParentID()), state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore()})
	rt.SetVMConfig(vm.Config{Debug: true, Tracer: tracer})
	clauseIndex := transactionTrace.ClauseIndex
	clause := txm.Clauses[clauseIndex]
	data, _ := hexutil.Decode(clause.Data)
	value := big.Int(clause.Value)
	c := tx.NewClause(clause.To).WithData(data).WithValue(&value)
	vmout := rt.ExecuteClause(c, clauseIndex, leftOverGas, txCtx)
	gasUsed := leftOverGas - vmout.LeftOverGas
	if err := rt.Seeker().Err(); err != nil {
		return nil, err
	}
	// Depending on the tracer type, format and return the output
	switch tracer := tracer.(type) {
	case *vm.StructLogger:
		return &ExecutionResult{
			Gas:         gasUsed,
			Failed:      vmout.VMErr != nil,
			ReturnValue: fmt.Sprintf("%x", vmout.Data),
			StructLogs:  FormatLogs(tracer.StructLogs()),
		}, nil

	case *tracers.Tracer:
		return tracer.GetResult()

	default:
		return nil, fmt.Errorf("bad tracer type %T", tracer)
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

func (t *Transactions) handleTraceTransaction(w http.ResponseWriter, req *http.Request) error {
	txID, err := thor.ParseBytes32(mux.Vars(req)["id"])
	if err != nil {
		return utils.BadRequest(err, "id")
	}
	transactionTrace := new(TransactionTrace)
	if err := utils.ParseJSON(req.Body, &transactionTrace); err != nil {
		return utils.BadRequest(err, "body")
	}
	blockID := t.chain.BestBlock().Header().ID()
	result, err := t.traceTransaction(req.Context(), txID, blockID, transactionTrace)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, result)
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
	h, err := t.getBlockHeader(req.URL.Query().Get("revision"))
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
	h, err := t.getBlockHeader(req.URL.Query().Get("revision"))
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

func (t *Transactions) getBlockHeader(revision string) (*block.Header, error) {
	if revision == "" || revision == "best" {
		return t.chain.BestBlock().Header(), nil
	}
	blkID, err := thor.ParseBytes32(revision)
	if err != nil {
		n, err := strconv.ParseUint(revision, 0, 0)
		if err != nil {
			return nil, err
		}
		if n > math.MaxUint32 {
			return nil, utils.BadRequest(errors.New("block number exceeded"), "revision")
		}
		b, err := t.chain.GetTrunkBlock(uint32(n))
		if err != nil {
			if t.chain.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return b.Header(), nil
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
	sub.Path("/{id}").Methods("GET").Queries("revision", "{revision}").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionByID))

	sub.Path("/{id}/receipt").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))
	sub.Path("/{id}/receipt").Methods("GET").Queries("revision", "{revision}").HandlerFunc(utils.WrapHandlerFunc(t.handleGetTransactionReceiptByID))

	sub.Path("/{id}/trace").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleTraceTransaction))

}
