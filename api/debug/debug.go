// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tracers"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
	"github.com/vechain/thor/v2/xenv"
)

const defaultMaxStorageResult = 1000

type Debug struct {
	repo              *chain.Repository
	stater            *state.Stater
	forkConfig        *thor.ForkConfig
	callGasLimit      uint64
	allowCustomTracer bool
	bft               bft.Committer
	allowedTracers    map[string]struct{}
	skipPoA           bool
	traceDataMaxSize  uint64
}

func New(
	repo *chain.Repository,
	stater *state.Stater,
	forkConfig *thor.ForkConfig,
	bft bft.Committer,
	callGaslimit uint64,
	allowCustomTracer bool,
	allowedTracers []string,
	soloMode bool,
	traceDataMaxSize uint64,
) *Debug {
	allowedMap := make(map[string]struct{})
	for _, t := range allowedTracers {
		allowedMap[t] = struct{}{}
	}

	return &Debug{
		repo,
		stater,
		forkConfig,
		callGaslimit,
		allowCustomTracer,
		bft,
		allowedMap,
		soloMode,
		traceDataMaxSize,
	}
}

// prepareClauseEnv prepares the runtime environment for the specified clause.
func (d *Debug) prepareClauseEnv(
	ctx context.Context,
	block *block.Block,
	txID thor.Bytes32,
	clauseIndex uint32,
) (*runtime.Runtime, *runtime.TransactionExecutor, thor.Bytes32, error) {
	rt, err := consensus.New(
		d.repo,
		d.stater,
		d.forkConfig,
	).NewRuntimeForReplay(block.Header(), d.skipPoA)
	if err != nil {
		return nil, nil, thor.Bytes32{}, err
	}

	var found bool
	txs := block.Transactions()
	for _, tx := range txs {
		if txID == tx.ID() {
			found = true
			if clauseIndex >= uint32(len(tx.Clauses())) {
				return nil, nil, thor.Bytes32{}, restutil.Forbidden(errors.New("clause index out of range"))
			}
		}
	}
	if !found {
		return nil, nil, thor.Bytes32{}, restutil.Forbidden(errors.New("transaction not found"))
	}

	for _, tx := range block.Transactions() {
		txExec, err := rt.PrepareTransaction(tx)
		if err != nil {
			return nil, nil, thor.Bytes32{}, err
		}
		clauseCounter := uint32(0)
		for txExec.HasNextClause() {
			if tx.ID() == txID && clauseIndex == clauseCounter {
				return rt, txExec, txID, nil
			}
			// Replaying the clauses before the target can cost up to a block's gas,
			// so each one stops when ctx does, as the traced clause does. A late
			// interrupt is harmless: it only cancels this clause's EVM, which is
			// not used again.
			exec, interrupt := txExec.PrepareNext()
			stop := context.AfterFunc(ctx, interrupt)
			interrupted, err := exec()
			stop()
			if interrupted {
				if ctx.Err() != nil {
					return nil, nil, thor.Bytes32{}, ctx.Err()
				}
				return nil, nil, thor.Bytes32{}, errors.New("execution interrupted")
			}
			if err != nil {
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

	// no env created, that means tx was reverted at an early clause
	return nil, nil, thor.Bytes32{}, restutil.Forbidden(errors.New("early reverted"))
}

// trace an existed clause
func (d *Debug) traceClause(ctx context.Context, tracer tracers.Tracer, native bool, block *block.Block, txID thor.Bytes32, clauseIndex uint32) (any, error) {
	rt, txExec, txID, err := d.prepareClauseEnv(ctx, block, txID, clauseIndex)
	if err != nil {
		return nil, err
	}

	var txIndex uint64 = math.MaxUint64
	for i, tx := range block.Transactions() {
		if tx.ID() == txID {
			txIndex = uint64(i)
			break
		}
	}
	tracer.SetContext(&tracers.Context{
		BlockID:     block.Header().ID(),
		BlockTime:   rt.Context().Time,
		TxID:        txID,
		TxIndex:     txIndex,
		ClauseIndex: clauseIndex,
		State:       rt.State(),
	})
	budget := newDataBudgetTracer(tracer, d.traceDataMaxSize)
	rt.SetVMConfig(vm.Config{Tracer: budget})

	exec, interrupt := txExec.PrepareNext()
	return traceExecution(ctx, budget, native, interrupt, exec)
}

// watchContext runs onDone if ctx is done before the returned release is
// called. release returns only once the watcher has exited, so onDone never
// runs after it.
func watchContext(ctx context.Context, onDone func()) (release func()) {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			onDone()
		case <-done:
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// traceExecution runs one traced clause execution under ctx and returns the
// tracer's result. When ctx is done it interrupts the EVM and stops the
// tracer; GetResult is reached only while ctx is still live, so a request
// whose deadline fired or whose client disconnected gets an error instead of
// a marshalled trace. A trace that exceeded its data budget is discarded the
// same way. native reports whether the tracer is implemented in Go.
func traceExecution(
	ctx context.Context,
	tracer *dataBudgetTracer,
	native bool,
	interrupt func(),
	exec func() (interrupted bool, err error),
) (any, error) {
	release := watchContext(ctx, func() {
		interrupt()
		tracer.Stop(ctx.Err())
	})
	interrupted, err := exec()
	release() // no tracer.Stop may run past this point

	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case tracer.err != nil:
		return nil, restutil.BadRequest(tracer.err)
	case interrupted:
		// Unreachable via the watchdog (ctx.Err() would be set above);
		// covers an interrupt() caller that is not tied to ctx.
		return nil, errors.New("execution interrupted")
	case err != nil:
		return nil, err
	}

	// A JS tracer's result function can be caller-supplied code that never
	// returns, so it runs under ctx too. Stop interrupts the JS runtime, which is
	// safe while GetResult runs. A native tracer's Stop is not, and its result is
	// bounded by the data budget.
	if native {
		return tracer.GetResult()
	}
	release = watchContext(ctx, func() { tracer.Stop(ctx.Err()) })
	res, err := tracer.GetResult()
	release()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return res, err
}

func (d *Debug) handleTraceClause(w http.ResponseWriter, req *http.Request) error {
	var opt api.TraceClauseOption
	if err := restutil.ParseJSON(req.Body, &opt); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}

	tracer, native, err := d.createTracer(opt.Name, opt.Config)
	if err != nil {
		return restutil.Forbidden(err)
	}

	block, txID, clauseIndex, err := d.parseTarget(opt.Target)
	if err != nil {
		return err
	}
	res, err := d.traceClause(req.Context(), tracer, native, block, txID, clauseIndex)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, res)
}

func (d *Debug) handleTraceCall(w http.ResponseWriter, req *http.Request) error {
	var opt api.TraceCallOption
	if err := restutil.ParseJSON(req.Body, &opt); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	revision, err := restutil.ParseRevision(req.URL.Query().Get("revision"), true)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, st, err := restutil.GetSummaryAndState(revision, d.repo, d.bft, d.stater, d.forkConfig)
	if err != nil {
		if d.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	tracer, native, err := d.createTracer(opt.Name, opt.Config)
	if err != nil {
		return restutil.Forbidden(err)
	}

	txCtx, gas, clause, err := d.handleTraceCallOption(&opt)
	if err != nil {
		return err
	}

	res, err := d.traceCall(req.Context(), tracer, native, summary.Header, st, txCtx, gas, clause)
	if err != nil {
		return err
	}

	return restutil.WriteJSON(w, res)
}

// createTracer returns the requested tracer and whether it is implemented in Go.
func (d *Debug) createTracer(name string, config json.RawMessage) (tracers.Tracer, bool, error) {
	tracerName := strings.TrimSpace(name)
	// compatible with old API specs
	if tracerName == "" {
		tracerName = "structLoggerTracer" // default to struct log tracer
	}

	// if it's builtin tracers
	if tracers.DefaultDirectory.Lookup(tracerName) {
		_, allowAll := d.allowedTracers["all"]
		// fail if the requested tracer is not allowed OR "all" not set
		if _, allowed := d.allowedTracers[tracerName]; !allowAll && !allowed {
			return nil, false, fmt.Errorf("creating tracer is not allowed: %s", name)
		}
		tracer, err := tracers.DefaultDirectory.New(tracerName, config, false)
		return tracer, tracers.DefaultDirectory.IsNative(tracerName), err
	}

	if d.allowCustomTracer {
		tracer, err := tracers.DefaultDirectory.New(tracerName, config, true)
		return tracer, false, err
	}

	return nil, false, errors.New("tracer is not defined")
}

func (d *Debug) traceCall(
	ctx context.Context,
	tracer tracers.Tracer,
	native bool,
	header *block.Header,
	st *state.State,
	txCtx *xenv.TransactionContext,
	gas uint64,
	clause *tx.Clause,
) (any, error) {
	signer, _ := header.Signer()

	rt := runtime.New(
		d.repo.NewChain(header.ParentID()),
		st,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
			BaseFee:     header.BaseFee(),
		},
		d.forkConfig)

	tracer.SetContext(&tracers.Context{
		BlockID:   header.ID(),
		BlockTime: header.Timestamp(),
		State:     st,
	})
	budget := newDataBudgetTracer(tracer, d.traceDataMaxSize)
	rt.SetVMConfig(vm.Config{Tracer: budget})

	exec, interrupt := rt.PrepareClause(clause, 0, gas, txCtx)
	return traceExecution(ctx, budget, native, interrupt, func() (bool, error) {
		_, interrupted, err := exec()
		return interrupted, err
	})
}

func (d *Debug) debugStorage(
	ctx context.Context,
	contractAddress thor.Address,
	block *block.Block,
	txID thor.Bytes32,
	clauseIndex uint32,
	keyStart []byte,
	maxResult int,
) (*api.StorageRangeResult, error) {
	rt, _, _, err := d.prepareClauseEnv(ctx, block, txID, clauseIndex)
	if err != nil {
		return nil, err
	}
	storageTrie, err := rt.State().BuildStorageTrie(contractAddress)
	if err != nil {
		return nil, err
	}
	return storageRangeAt(storageTrie, keyStart, maxResult)
}

func storageRangeAt(t *muxdb.Trie, start []byte, maxResult int) (*api.StorageRangeResult, error) {
	it := trie.NewIterator(t.NodeIterator(start, 0))
	result := api.StorageRangeResult{Storage: api.StorageMap{}}
	for i := 0; i < maxResult && it.Next(); i++ {
		_, content, _, err := rlp.Split(it.Value)
		if err != nil {
			return nil, err
		}
		v := thor.BytesToBytes32(content)
		e := api.StorageEntry{Value: &v}
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
	var opt api.StorageRangeOption
	if err := restutil.ParseJSON(req.Body, &opt); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}

	if opt.MaxResult > defaultMaxStorageResult {
		return restutil.BadRequest(errors.Errorf("maxResult: exceeds limit of %d", defaultMaxStorageResult))
	}

	if opt.MaxResult == 0 {
		opt.MaxResult = defaultMaxStorageResult
	}

	blockID, txIndex, clauseIndex, err := d.parseTarget(opt.Target)
	if err != nil {
		return err
	}
	var keyStart []byte
	if opt.KeyStart != "" {
		k, err := hexutil.Decode(opt.KeyStart)
		if err != nil {
			return restutil.BadRequest(errors.New("keyStart: invalid format"))
		}
		keyStart = k
	}
	res, err := d.debugStorage(req.Context(), opt.Address, blockID, txIndex, clauseIndex, keyStart, opt.MaxResult)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, res)
}

func (d *Debug) parseTarget(target string) (block *block.Block, txID thor.Bytes32, clauseIndex uint32, err error) {
	// target can be `${blockID}/${txID|txIndex}/${clauseIndex}` or `${txID}/${clauseIndex}`
	parts := strings.Split(target, "/")
	if len(parts) != 3 && len(parts) != 2 {
		return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.New("target:" + target + " unsupported"))
	}

	if len(parts) == 2 {
		txID, err = thor.ParseBytes32(parts[0])
		if err != nil {
			return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, "target([0]"))
		}
		bestChain := d.repo.NewBestChain()
		txMeta, err := bestChain.GetTransactionMeta(txID)
		if err != nil {
			if d.repo.IsNotFound(err) {
				return nil, thor.Bytes32{}, 0, restutil.Forbidden(errors.New("transaction not found"))
			}
			return nil, thor.Bytes32{}, 0, err
		}
		block, err = bestChain.GetBlock(txMeta.BlockNum)
		if err != nil {
			return nil, thor.Bytes32{}, 0, err
		}
	} else {
		blockID, err := thor.ParseBytes32(parts[0])
		if err != nil {
			return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, "target[0]"))
		}
		block, err = d.repo.GetBlock(blockID)
		if err != nil {
			if d.repo.IsNotFound(err) {
				return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, "target[0]"))
			}
			return nil, thor.Bytes32{}, 0, err
		}
		if len(parts[1]) == 64 || len(parts[1]) == 66 {
			txID, err = thor.ParseBytes32(parts[1])
			if err != nil {
				return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, "target[1]"))
			}

			var found bool
			for _, tx := range block.Transactions() {
				if tx.ID() == txID {
					found = true
					break
				}
			}
			if !found {
				return nil, thor.Bytes32{}, 0, restutil.Forbidden(errors.New("transaction not found"))
			}
		} else {
			i, err := strconv.ParseUint(parts[1], 0, 0)
			if err != nil {
				return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, "target[1]"))
			}
			if i >= uint64(len(block.Transactions())) {
				return nil, thor.Bytes32{}, 0, restutil.Forbidden(errors.New("tx index out of range"))
			}
			txID = block.Transactions()[i].ID()
		}
	}

	i, err := strconv.ParseUint(parts[len(parts)-1], 0, 0)
	if err != nil {
		return nil, thor.Bytes32{}, 0, restutil.BadRequest(errors.WithMessage(err, fmt.Sprintf("target[%d]", len(parts)-1)))
	} else if i > math.MaxUint32 {
		return nil, thor.Bytes32{}, 0, restutil.BadRequest(fmt.Errorf("invalid target[%d]", len(parts)-1))
	}
	clauseIndex = uint32(i)
	return
}

func (d *Debug) handleTraceCallOption(opt *api.TraceCallOption) (*xenv.TransactionContext, uint64, *tx.Clause, error) {
	gas := opt.Gas
	if opt.Gas > d.callGasLimit {
		return nil, 0, nil, restutil.Forbidden(errors.New("gas: exceeds limit"))
	} else if opt.Gas == 0 {
		gas = d.callGasLimit
	}

	txCtx := xenv.TransactionContext{
		ClauseCount: 1,
		Expiration:  opt.Expiration,
	}
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

	if len(opt.BlockRef) > 0 {
		blkRef, err := restutil.ParseBlockRef(opt.BlockRef)
		if err != nil {
			return nil, 0, nil, err
		}
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
			return nil, 0, nil, restutil.BadRequest(errors.WithMessage(err, "data"))
		}
	}

	clause := tx.NewClause(opt.To).WithValue(value).WithData(data)
	return &txCtx, gas, clause, nil
}

func (d *Debug) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/tracers").
		Methods(http.MethodPost).
		Name("POST /debug/tracers").
		HandlerFunc(restutil.WrapHandlerFunc(d.handleTraceClause))
	sub.Path("/tracers/call").
		Methods(http.MethodPost).
		Name("POST /debug/tracers/call").
		HandlerFunc(restutil.WrapHandlerFunc(d.handleTraceCall))
	sub.Path("/storage-range").
		Methods(http.MethodPost).
		Name("POST /debug/storage-range").
		HandlerFunc(restutil.WrapHandlerFunc(d.handleDebugStorage))
}
