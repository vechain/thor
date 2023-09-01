// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"context"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tracers"
	"github.com/vechain/thor/tracers/logger"
	"github.com/vechain/thor/trie"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"github.com/vechain/thor/xenv"
)

var devNetGenesisID = genesis.NewDevnet().ID()

type Debug struct {
	repo              *chain.Repository
	stater            *state.Stater
	forkConfig        thor.ForkConfig
	callGasLimit      uint64
	allowCustomTracer bool
}

func New(repo *chain.Repository, stater *state.Stater, forkConfig thor.ForkConfig, callGaslimit uint64, allowCustomTracer bool) *Debug {
	return &Debug{
		repo,
		stater,
		forkConfig,
		callGaslimit,
		allowCustomTracer,
	}
}

func (d *Debug) prepareClauseEnv(ctx context.Context, blockID thor.Bytes32, txIndex uint64, clauseIndex uint64) (*runtime.Runtime, *runtime.TransactionExecutor, thor.Bytes32, error) {
	block, err := d.repo.GetBlock(blockID)
	if err != nil {
		if d.repo.IsNotFound(err) {
			return nil, nil, thor.Bytes32{}, utils.Forbidden(errors.New("block not found"))
		}
		return nil, nil, thor.Bytes32{}, err
	}
	txs := block.Transactions()
	if txIndex >= uint64(len(txs)) {
		return nil, nil, thor.Bytes32{}, utils.Forbidden(errors.New("tx index out of range"))
	}
	txID := txs[txIndex].ID()
	if clauseIndex >= uint64(len(txs[txIndex].Clauses())) {
		return nil, nil, thor.Bytes32{}, utils.Forbidden(errors.New("clause index out of range"))
	}
	skipPoA := d.repo.GenesisBlock().Header().ID() == devNetGenesisID
	rt, err := consensus.New(
		d.repo,
		d.stater,
		d.forkConfig,
	).NewRuntimeForReplay(block.Header(), skipPoA)
	if err != nil {
		return nil, nil, thor.Bytes32{}, err
	}
	for i, tx := range txs {
		if uint64(i) > txIndex {
			break
		}
		txExec, err := rt.PrepareTransaction(tx)
		if err != nil {
			return nil, nil, thor.Bytes32{}, err
		}
		clauseCounter := uint64(0)
		for txExec.HasNextClause() {
			if txIndex == uint64(i) && clauseIndex == clauseCounter {
				return rt, txExec, txID, nil
			}
			exec, _ := txExec.PrepareNext()
			if _, _, err := exec(); err != nil {
				return nil, nil, thor.Bytes32{}, err
			}
			clauseCounter++
		}
		if _, err := txExec.Finalize(); err != nil {
			return nil, nil, thor.Bytes32{}, err
		}
		select {
		case <-ctx.Done():
			return nil, nil, thor.Bytes32{}, ctx.Err()
		default:
		}
	}
	return nil, nil, thor.Bytes32{}, utils.Forbidden(errors.New("early reverted"))
}

// trace an existed clause
func (d *Debug) traceClause(ctx context.Context, tracer tracers.Tracer, blockID thor.Bytes32, txIndex uint64, clauseIndex uint64) (interface{}, error) {
	rt, txExec, txID, err := d.prepareClauseEnv(ctx, blockID, txIndex, clauseIndex)
	if err != nil {
		return nil, err
	}

	tracer.SetContext(&tracers.Context{
		BlockID:     blockID,
		BlockTime:   rt.Context().Time,
		TxID:        txID,
		TxIndex:     int(txIndex),
		ClauseIndex: int(clauseIndex),
		State:       rt.State(),
	})
	rt.SetVMConfig(vm.Config{Tracer: tracer})
	errCh := make(chan error, 1)
	exec, interrupt := txExec.PrepareNext()
	go func() {
		_, _, err := exec()
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		err := ctx.Err()
		tracer.Stop(err)
		interrupt()
		return nil, err
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	}
	return tracer.GetResult()
}

func (d *Debug) handleTraceClause(w http.ResponseWriter, req *http.Request) error {
	var opt TraceClauseOption
	if err := utils.ParseJSON(req.Body, &opt); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	var tracer tracers.Tracer
	if opt.Name == "" {
		tr, err := logger.NewStructLogger(opt.Config)
		if err != nil {
			return utils.Forbidden(err)
		}
		tracer = tr
	} else {
		tr, err := tracers.DefaultDirectory.New(opt.Name, opt.Config, d.allowCustomTracer)
		if err != nil {
			return utils.Forbidden(err)
		}
		tracer = tr
	}
	blockID, txIndex, clauseIndex, err := d.parseTarget(opt.Target)
	if err != nil {
		return err
	}
	res, err := d.traceClause(req.Context(), tracer, blockID, txIndex, clauseIndex)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, res)
}

func (d *Debug) handleTraceCall(w http.ResponseWriter, req *http.Request) error {
	var opt TraceCallOption
	if err := utils.ParseJSON(req.Body, &opt); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}

	summary, err := d.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}

	var tracer tracers.Tracer
	if opt.Name == "" {
		tr, err := logger.NewStructLogger(opt.Config)
		if err != nil {
			return utils.Forbidden(err)
		}
		tracer = tr
	} else {
		tr, err := tracers.DefaultDirectory.New(opt.Name, opt.Config, d.allowCustomTracer)
		if err != nil {
			return utils.Forbidden(err)
		}
		tracer = tr
	}

	txCtx, gas, clause, err := d.handleTraceCallOption(&opt)
	if err != nil {
		return err
	}

	res, err := d.traceCall(req.Context(), tracer, summary, txCtx, gas, clause)
	if err != nil {
		return err
	}

	return utils.WriteJSON(w, res)
}

func (d *Debug) traceCall(ctx context.Context, tracer tracers.Tracer, summary *chain.BlockSummary, txCtx *xenv.TransactionContext, gas uint64, clause *tx.Clause) (interface{}, error) {
	header := summary.Header
	state := d.stater.NewState(header.StateRoot(), header.Number(), summary.Conflicts, summary.SteadyNum)
	signer, _ := header.Signer()
	rt := runtime.New(
		d.repo.NewChain(header.ID()),
		state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
		},
		d.forkConfig)

	tracer.SetContext(&tracers.Context{
		BlockID:   summary.Header.ID(),
		BlockTime: summary.Header.Timestamp(),
		State:     state,
	})
	rt.SetVMConfig(vm.Config{Tracer: tracer})

	errCh := make(chan error, 1)
	exec, interrupt := rt.PrepareClause(clause, 0, gas, txCtx)
	go func() {
		_, _, err := exec()
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		err := ctx.Err()
		tracer.Stop(err)
		interrupt()
		return nil, err
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	}
	return tracer.GetResult()
}

func (d *Debug) debugStorage(ctx context.Context, contractAddress thor.Address, blockID thor.Bytes32, txIndex uint64, clauseIndex uint64, keyStart []byte, maxResult int) (*StorageRangeResult, error) {
	rt, _, _, err := d.prepareClauseEnv(ctx, blockID, txIndex, clauseIndex)
	if err != nil {
		return nil, err
	}
	storageTrie, err := rt.State().BuildStorageTrie(contractAddress)
	if err != nil {
		return nil, err
	}
	return storageRangeAt(storageTrie, keyStart, maxResult)
}

func storageRangeAt(t *muxdb.Trie, start []byte, maxResult int) (*StorageRangeResult, error) {
	it := trie.NewIterator(t.NodeIterator(start, 0))
	result := StorageRangeResult{Storage: StorageMap{}}
	for i := 0; i < maxResult && it.Next(); i++ {
		_, content, _, err := rlp.Split(it.Value)
		if err != nil {
			return nil, err
		}
		v := thor.BytesToBytes32(content)
		e := StorageEntry{Value: &v}
		preimage := thor.BytesToBytes32(it.Meta)
		e.Key = &preimage
		result.Storage[thor.BytesToBytes32(it.Key).String()] = e
	}
	if it.Next() {
		next := thor.BytesToBytes32(it.Key)
		result.NextKey = &next
	}
	return &result, nil
}

func (d *Debug) handleDebugStorage(w http.ResponseWriter, req *http.Request) error {
	var opt StorageRangeOption
	if err := utils.ParseJSON(req.Body, &opt); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	blockID, txIndex, clauseIndex, err := d.parseTarget(opt.Target)
	if err != nil {
		return err
	}
	var keyStart []byte
	if opt.KeyStart != "" {
		k, err := hexutil.Decode(opt.KeyStart)
		if err != nil {
			return utils.BadRequest(errors.New("keyStart: invalid format"))
		}
		keyStart = k
	}
	res, err := d.debugStorage(req.Context(), opt.Address, blockID, txIndex, clauseIndex, keyStart, opt.MaxResult)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, res)
}

func (d *Debug) parseTarget(target string) (blockID thor.Bytes32, txIndex uint64, clauseIndex uint64, err error) {
	parts := strings.Split(target, "/")
	if len(parts) != 3 {
		return thor.Bytes32{}, 0, 0, utils.BadRequest(errors.New("target:" + target + " unsupported"))
	}
	blockID, err = thor.ParseBytes32(parts[0])
	if err != nil {
		return thor.Bytes32{}, 0, 0, utils.BadRequest(errors.WithMessage(err, "target[0]"))
	}
	if len(parts[1]) == 64 || len(parts[1]) == 66 {
		txID, err := thor.ParseBytes32(parts[1])
		if err != nil {
			return thor.Bytes32{}, 0, 0, utils.BadRequest(errors.WithMessage(err, "target[1]"))
		}

		txMeta, err := d.repo.NewChain(blockID).GetTransactionMeta(txID)
		if err != nil {
			if d.repo.IsNotFound(err) {
				return thor.Bytes32{}, 0, 0, utils.Forbidden(errors.New("transaction not found"))
			}
			return thor.Bytes32{}, 0, 0, err
		}
		txIndex = txMeta.Index
	} else {
		i, err := strconv.ParseUint(parts[1], 0, 0)
		if err != nil {
			return thor.Bytes32{}, 0, 0, utils.BadRequest(errors.WithMessage(err, "target[1]"))
		}
		txIndex = i
	}
	clauseIndex, err = strconv.ParseUint(parts[2], 0, 0)
	if err != nil {
		return thor.Bytes32{}, 0, 0, utils.BadRequest(errors.WithMessage(err, "target[2]"))
	}
	return
}

func (d *Debug) handleRevision(revision string) (*chain.BlockSummary, error) {
	if revision == "" || revision == "best" {
		return d.repo.BestBlockSummary(), nil
	}
	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		summary, err := d.repo.GetBlockSummary(blockID)
		if err != nil {
			if d.repo.IsNotFound(err) {
				return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
			}
			return nil, err
		}
		return summary, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	if n > math.MaxUint32 {
		return nil, utils.BadRequest(errors.WithMessage(errors.New("block number out of max uint32"), "revision"))
	}
	summary, err := d.repo.NewBestChain().GetBlockSummary(uint32(n))
	if err != nil {
		if d.repo.IsNotFound(err) {
			return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return nil, err
	}
	return summary, nil
}

func (d *Debug) handleTraceCallOption(opt *TraceCallOption) (*xenv.TransactionContext, uint64, *tx.Clause, error) {
	gas := opt.Gas
	if opt.Gas > d.callGasLimit {
		return nil, 0, nil, utils.Forbidden(errors.New("gas: exceeds limit"))
	} else if opt.Gas == 0 {
		gas = d.callGasLimit
	}

	var txCtx xenv.TransactionContext
	if opt.GasPrice == nil {
		txCtx.GasPrice = new(big.Int)
	} else {
		txCtx.GasPrice = (*big.Int)(opt.GasPrice)
	}
	if opt.Caller == nil {
		txCtx.Origin = thor.Address{}
	} else {
		txCtx.Origin = *opt.Caller
	}
	if opt.GasPayer == nil {
		txCtx.GasPayer = thor.Address{}
	} else {
		txCtx.GasPayer = *opt.GasPayer
	}
	if opt.ProvedWork == nil {
		txCtx.ProvedWork = new(big.Int)
	} else {
		txCtx.ProvedWork = (*big.Int)(opt.ProvedWork)
	}
	txCtx.Expiration = opt.Expiration

	if len(opt.BlockRef) > 0 {
		blockRef, err := hexutil.Decode(opt.BlockRef)
		if err != nil {
			return nil, 0, nil, errors.WithMessage(err, "blockRef")
		}
		if len(blockRef) != 8 {
			return nil, 0, nil, errors.New("blockRef: invalid length")
		}
		var blkRef tx.BlockRef
		copy(blkRef[:], blockRef[:])
		txCtx.BlockRef = blkRef
	}

	var value *big.Int
	if opt.Value == nil {
		value = new(big.Int)
	} else {
		value = (*big.Int)(opt.Value)
	}

	var data []byte
	var err error
	if opt.Data != "" {
		data, err = hexutil.Decode(opt.Data)
		if err != nil {
			return nil, 0, nil, utils.BadRequest(errors.WithMessage(err, "data"))
		}
	}

	clause := tx.NewClause(opt.To).WithValue(value).WithData(data)
	return &txCtx, gas, clause, nil
}

func (d *Debug) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/tracers").Methods(http.MethodPost).HandlerFunc(utils.WrapHandlerFunc(d.handleTraceClause))
	sub.Path("/tracers/call").Methods(http.MethodPost).HandlerFunc(utils.WrapHandlerFunc(d.handleTraceCall))
	sub.Path("/storage-range").Methods(http.MethodPost).HandlerFunc(utils.WrapHandlerFunc(d.handleDebugStorage))

}
