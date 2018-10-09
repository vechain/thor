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
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/vm"
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

//trace an existed transaction
func (d *Debug) traceTransaction(ctx context.Context, tracer vm.Tracer, blockID thor.Bytes32, txIndex uint64, clauseIndex uint64) (interface{}, error) {
	block, err := d.chain.GetBlock(blockID)
	if err != nil {
		if d.chain.IsNotFound(err) {
			return nil, utils.Forbidden(errors.New("block not found"))
		}
		return nil, err
	}
	txs := block.Transactions()
	if txIndex >= uint64(len(txs)) {
		return nil, utils.Forbidden(errors.New("tx index out of range"))
	}
	if clauseIndex >= uint64(len(txs[txIndex].Clauses())) {
		return nil, utils.Forbidden(errors.New("clause index out of range"))
	}

	rt, err := consensus.New(d.chain, d.stateC).NewRuntimeForReplay(block.Header())
	if err != nil {
		return nil, err
	}

	for i, tx := range txs {
		if uint64(i) > txIndex {
			break
		}
		txExec, err := rt.PrepareTransaction(tx)
		if err != nil {
			return nil, err
		}
		clauseCounter := uint64(0)
		for txExec.HasNextClause() {
			isTarget := txIndex == uint64(i) && clauseIndex == clauseCounter
			if isTarget {
				rt.SetVMConfig(vm.Config{Debug: true, Tracer: tracer})
			}
			gasUsed, output, err := txExec.NextClause()
			if err != nil {
				return nil, err
			}
			if isTarget {
				switch tr := tracer.(type) {
				case *vm.StructLogger:
					return &ExecutionResult{
						Gas:         gasUsed,
						Failed:      output.VMErr != nil,
						ReturnValue: hexutil.Encode(output.Data),
						StructLogs:  formatLogs(tr.StructLogs()),
					}, nil
				case *tracers.Tracer:
					return tr.GetResult()
				default:
					return nil, fmt.Errorf("bad tracer type %T", tracer)
				}
			}
			clauseCounter++
		}
		if _, err := txExec.Finalize(); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return nil, utils.Forbidden(errors.New("early reverted"))
}

func (d *Debug) handleTraceTransaction(w http.ResponseWriter, req *http.Request) error {
	var opt *TracerOption
	if err := utils.ParseJSON(req.Body, &opt); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	var tracer vm.Tracer
	if opt.Name == "" {
		tracer = vm.NewStructLogger(nil)
	} else {
		name := opt.Name
		if !strings.HasSuffix(name, "Tracer") {
			name += "Tracer"
		}
		code, ok := tracers.CodeByName(name)
		if !ok {
			return utils.BadRequest(errors.New("name: unsupported tracer"))
		}
		tr, err := tracers.New(code)
		if err != nil {
			return err
		}
		tracer = tr
	}
	parts := strings.Split(opt.Target, "/")
	if len(parts) != 3 {
		return utils.BadRequest(errors.New("target:" + opt.Target + " unsupported"))
	}
	blockID, err := thor.ParseBytes32(parts[0])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "target[0]"))
	}
	var txIndex uint64
	if len(parts[1]) == 64 || len(parts[1]) == 66 {
		txID, err := thor.ParseBytes32(parts[1])
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "target[1]"))
		}
		txMeta, err := d.chain.GetTransactionMeta(txID, blockID)
		if err != nil {
			if d.chain.IsNotFound(err) {
				return utils.Forbidden(errors.New("transaction not found"))
			}
			return err
		}
		txIndex = txMeta.Index
	} else {
		i, err := strconv.ParseUint(parts[1], 0, 0)
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "target[1]"))
		}
		txIndex = i
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
