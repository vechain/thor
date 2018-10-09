// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

type Debug struct {
	chain  *chain.Chain
	stateC *state.Creator
}

func New(chain *chain.Chain, stateC *state.Creator) *Debug {
	return &Debug{
		chain,
		stateC,
	}
}

func (d *Debug) computeTxEnv(ctx context.Context, block *block.Block, txIndex uint64) (*runtime.Runtime, *state.State, error) {
	parentHeader, err := d.chain.GetBlockHeader(block.Header().ParentID())
	if err != nil {
		return nil, nil, err
	}
	st, err := d.stateC.NewState(parentHeader.StateRoot())
	if err != nil {
		return nil, nil, err
	}
	signer, err := parentHeader.Signer()
	if err != nil {
		return nil, nil, err
	}
	blockContext := &xenv.BlockContext{
		Beneficiary: parentHeader.Beneficiary(),
		Signer:      signer,
		Number:      parentHeader.Number(),
		Time:        parentHeader.Timestamp(),
		GasLimit:    parentHeader.GasLimit(),
		TotalScore:  parentHeader.TotalScore(),
	}
	rt := runtime.New(d.chain.NewSeeker(parentHeader.ID()), st, blockContext)
	executeTx := func(tx *tx.Transaction) error {
		_, err := rt.ExecuteTransaction(tx)
		return err
	}
	for idx, tx := range block.Transactions() {
		if idx == int(txIndex) {
			return rt, st, nil
		}
		errExecuteTx := make(chan error, 1)
		go func() {
			errExecuteTx <- executeTx(tx)
		}()
		select {
		case err := <-errExecuteTx:
			if err != nil {
				return nil, nil, err
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	return nil, nil, fmt.Errorf("tx index %d out of range for block %x", txIndex, block.Header().ID())
}

//trace an existed transaction
func (d *Debug) traceTransaction(ctx context.Context, tracer vm.Tracer, blockID thor.Bytes32, txIndex uint64, clauseIndex uint64) (interface{}, error) {
	tx, err := d.chain.GetTransaction(blockID, txIndex)
	if err != nil {
		return nil, err
	}
	txSinger, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	block, err := d.chain.GetBlock(blockID)
	if err != nil {
		return nil, err
	}
	rt, st, err := d.computeTxEnv(ctx, block, txIndex)
	if err != nil {
		return nil, err
	}
	rt.SetVMConfig(vm.Config{Debug: true, Tracer: tracer})
	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	txCtx := &xenv.TransactionContext{
		ID:         tx.ID(),
		Origin:     txSinger,
		GasPrice:   tx.GasPrice(baseGasPrice),
		ProvedWork: tx.ProvedWork(block.Header().Number(), rt.Seeker().GetID),
		BlockRef:   tx.BlockRef(),
		Expiration: tx.Expiration()}
	clauses := tx.Clauses()
	leftOverGas := tx.Gas()
	gasUsed := uint64(0)
	for i, clause := range clauses {
		vmout := make(chan *runtime.Output, 1)
		exec, interrupt := rt.PrepareClause(clause, uint32(i), leftOverGas, txCtx)
		go func() {
			o, _ := exec()
			vmout <- o
		}()
		select {
		case <-ctx.Done():
			interrupt()
			return nil, ctx.Err()
		case vo := <-vmout:
			if vo.VMErr != nil {
				return nil, vo.VMErr
			}
			if err := rt.Seeker().Err(); err != nil {
				return nil, err
			}
			if err := st.Err(); err != nil {
				return nil, err
			}
			gasUsed += leftOverGas - vo.LeftOverGas
			leftOverGas = vo.LeftOverGas
			if i == int(clauseIndex) {
				switch tr := tracer.(type) {
				case *vm.StructLogger:
					return &ExecutionResult{
						Gas:         gasUsed,
						Failed:      vo.VMErr != nil,
						ReturnValue: hexutil.Encode(vo.Data),
						StructLogs:  formatLogs(tr.StructLogs()),
					}, nil
				case *tracers.Tracer:
					return tr.GetResult()
				default:
					return nil, fmt.Errorf("bad tracer type %T", tracer)
				}
			}
		}
	}
	return nil, fmt.Errorf("clause index %d out of range for tx %x", clauseIndex, tx.ID())
}

func (d *Debug) handleTraceTransaction(w http.ResponseWriter, req *http.Request) error {
	var traceop *TracerOption
	if err := utils.ParseJSON(req.Body, &traceop); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	var tracer vm.Tracer
	if traceop.Name == "" {
		tracer = vm.NewStructLogger(nil)
	} else {
		tr, err := tracers.New(traceop.Name)
		if err != nil {
			return err
		}
		tracer = tr
	}
	parts := strings.Split(traceop.Target, "/")
	if len(parts) != 3 {
		return utils.BadRequest(errors.New("target:" + traceop.Target + " unsupported"))
	}
	blockID, err := thor.ParseBytes32(parts[0])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "target[0]"))
	}
	txIndex, err := strconv.ParseUint(parts[1], 0, 0)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "target[1]"))
	}
	clauseIndex, err := strconv.ParseUint(parts[2], 0, 0)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "target[2]"))
	}

	res, err := d.traceTransaction(req.Context(), tracer, blockID, txIndex, clauseIndex)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, res)
}

func (d *Debug) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/tracers").Methods(http.MethodPost).HandlerFunc(utils.WrapHandlerFunc(d.handleTraceTransaction))
}
