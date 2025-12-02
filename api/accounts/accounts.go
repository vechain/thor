// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

type Accounts struct {
	repo              *chain.Repository
	stater            *state.Stater
	callGasLimit      uint64
	forkConfig        *thor.ForkConfig
	bft               bft.Committer
	enabledDeprecated bool
}

func New(
	repo *chain.Repository,
	stater *state.Stater,
	callGasLimit uint64,
	forkConfig *thor.ForkConfig,
	bft bft.Committer,
	enabledDeprecated bool,
) *Accounts {
	return &Accounts{
		repo,
		stater,
		callGasLimit,
		forkConfig,
		bft,
		enabledDeprecated,
	}
}

func (a *Accounts) getCode(addr thor.Address, state *state.State) ([]byte, error) {
	code, err := state.GetCode(addr)
	if err != nil {
		return nil, err
	}
	return code, nil
}

func (a *Accounts) handleGetCode(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "address"))
	}
	revision, err := restutil.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}

	_, st, err := restutil.GetSummaryAndState(revision, a.repo, a.bft, a.stater, a.forkConfig)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	code, err := a.getCode(addr, st)
	if err != nil {
		return err
	}

	return restutil.WriteJSON(w, &api.GetCodeResult{Code: hexutil.Encode(code)})
}

func (a *Accounts) getAccount(addr thor.Address, header *block.Header, state *state.State) (*api.Account, error) {
	b, err := state.GetBalance(addr)
	if err != nil {
		return nil, err
	}
	code, err := state.GetCode(addr)
	if err != nil {
		return nil, err
	}
	energy, err := builtin.Energy.Native(state, header.Timestamp(), a.repo.EnergyStopTimeFunc(header.ID(), header.Timestamp())).Get(addr)
	if err != nil {
		return nil, err
	}

	return &api.Account{
		Balance: (*math.HexOrDecimal256)(b),
		Energy:  (*math.HexOrDecimal256)(energy),
		HasCode: len(code) != 0,
	}, nil
}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "address"))
	}
	revision, err := restutil.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}

	summary, st, err := restutil.GetSummaryAndState(revision, a.repo, a.bft, a.stater, a.forkConfig)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	acc, err := a.getAccount(addr, summary.Header, st)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, acc)
}

func (a *Accounts) getStorage(addr thor.Address, key thor.Bytes32, state *state.State) (thor.Bytes32, error) {
	storage, err := state.GetStorage(addr, key)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return storage, nil
}

func (a *Accounts) parseStorageRequest(routerVars map[string]string, queryParams url.Values) (thor.Address, thor.Bytes32, *state.State, error) {
	addr, err := thor.ParseAddress(routerVars["address"])
	if err != nil {
		return thor.Address{}, thor.Bytes32{}, nil, restutil.BadRequest(errors.WithMessage(err, "address"))
	}
	key, err := thor.ParseBytes32(routerVars["key"])
	if err != nil {
		return thor.Address{}, thor.Bytes32{}, nil, restutil.BadRequest(errors.WithMessage(err, "key"))
	}
	revision, err := restutil.ParseRevision(queryParams.Get("revision"), false)
	if err != nil {
		return thor.Address{}, thor.Bytes32{}, nil, restutil.BadRequest(errors.WithMessage(err, "revision"))
	}

	_, st, err := restutil.GetSummaryAndState(revision, a.repo, a.bft, a.stater, a.forkConfig)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return thor.Address{}, thor.Bytes32{}, nil, restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return thor.Address{}, thor.Bytes32{}, nil, err
	}

	return addr, key, st, nil
}

func (a *Accounts) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	addr, key, st, err := a.parseStorageRequest(mux.Vars(req), req.URL.Query())
	if err != nil {
		return err
	}

	storage, err := a.getStorage(addr, key, st)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, &api.GetStorageResult{Value: storage.String()})
}

func (a *Accounts) getRawStorage(addr thor.Address, key thor.Bytes32, state *state.State) ([]byte, error) {
	storage, err := state.GetRawStorage(addr, key)
	if err != nil {
		return nil, err
	}
	return storage, nil
}

func (a *Accounts) handleGetRawStorage(w http.ResponseWriter, req *http.Request) error {
	addr, key, st, err := a.parseStorageRequest(mux.Vars(req), req.URL.Query())
	if err != nil {
		return err
	}

	storage, err := a.getRawStorage(addr, key, st)
	if err != nil {
		return err
	}

	return restutil.WriteJSON(w, &api.GetStorageResult{Value: hexutil.Encode(storage)})
}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	callData := &api.CallData{}
	if err := restutil.ParseJSON(req.Body, &callData); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	revision, err := restutil.ParseRevision(req.URL.Query().Get("revision"), true)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, st, err := restutil.GetSummaryAndState(revision, a.repo, a.bft, a.stater, a.forkConfig)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	var addr *thor.Address
	if mux.Vars(req)["address"] != "" {
		address, err := thor.ParseAddress(mux.Vars(req)["address"])
		if err != nil {
			return restutil.BadRequest(errors.WithMessage(err, "address"))
		}
		addr = &address
	}
	batchCallData := &api.BatchCallData{
		Clauses: api.Clauses{
			&api.Clause{
				To:    addr,
				Value: callData.Value,
				Data:  callData.Data,
			},
		},
		Gas:      callData.Gas,
		GasPrice: callData.GasPrice,
		Caller:   callData.Caller,
	}
	results, err := a.batchCall(req.Context(), batchCallData, summary.Header, st)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, results[0])
}

func (a *Accounts) handleCallBatchCode(w http.ResponseWriter, req *http.Request) error {
	var batchCallData api.BatchCallData
	if err := restutil.ParseJSON(req.Body, &batchCallData); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	// reject null element in clauses, {} will be unmarshaled to default value and will be accepted/handled by the runtime
	for i, clause := range batchCallData.Clauses {
		if clause == nil {
			return restutil.BadRequest(fmt.Errorf("clauses[%d]: null not allowed", i))
		}
	}
	revision, err := restutil.ParseRevision(req.URL.Query().Get("revision"), true)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, st, err := restutil.GetSummaryAndState(revision, a.repo, a.bft, a.stater, a.forkConfig)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	results, err := a.batchCall(req.Context(), &batchCallData, summary.Header, st)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, results)
}

func (a *Accounts) batchCall(
	ctx context.Context,
	batchCallData *api.BatchCallData,
	header *block.Header,
	st *state.State,
) (results api.BatchCallResults, err error) {
	txCtx, gas, clauses, err := a.handleBatchCallData(batchCallData)
	if err != nil {
		return nil, err
	}

	signer, _ := header.Signer()
	rt := runtime.New(a.repo.NewChain(header.ParentID()), st,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
			BaseFee:     header.BaseFee(),
		},
		a.forkConfig)
	results = make(api.BatchCallResults, 0)
	resultCh := make(chan any, 1)
	for i, clause := range clauses {
		exec, interrupt := rt.PrepareClause(clause, uint32(i), gas, txCtx)
		go func() {
			out, _, err := exec()
			if err != nil {
				resultCh <- err
			}
			resultCh <- out
		}()
		select {
		case <-ctx.Done():
			interrupt()
			return nil, ctx.Err()
		case result := <-resultCh:
			switch v := result.(type) {
			case error:
				return nil, v
			case *runtime.Output:
				results = append(results, api.ConvertCallResultWithInputGas(v, gas))
				if v.VMErr != nil {
					return results, nil
				}
				gas = v.LeftOverGas
			}
		}
	}
	return results, nil
}

func (a *Accounts) handleBatchCallData(batchCallData *api.BatchCallData) (txCtx *xenv.TransactionContext, gas uint64, clauses []*tx.Clause, err error) {
	if batchCallData.Gas > a.callGasLimit {
		return nil, 0, nil, restutil.Forbidden(errors.New("gas: exceeds limit"))
	} else if batchCallData.Gas == 0 {
		gas = a.callGasLimit
	} else {
		gas = batchCallData.Gas
	}

	txCtx = &xenv.TransactionContext{
		ClauseCount: uint32(len(batchCallData.Clauses)),
		Expiration:  batchCallData.Expiration,
	}

	if batchCallData.GasPrice == nil {
		txCtx.GasPrice = new(big.Int)
	} else {
		txCtx.GasPrice = (*big.Int)(batchCallData.GasPrice)
	}
	if batchCallData.Caller == nil {
		txCtx.Origin = thor.Address{}
	} else {
		txCtx.Origin = *batchCallData.Caller
	}
	if batchCallData.GasPayer == nil {
		txCtx.GasPayer = thor.Address{}
	} else {
		txCtx.GasPayer = *batchCallData.GasPayer
	}
	if batchCallData.ProvedWork == nil {
		txCtx.ProvedWork = new(big.Int)
	} else {
		txCtx.ProvedWork = (*big.Int)(batchCallData.ProvedWork)
	}

	if len(batchCallData.BlockRef) > 0 {
		blockRef, err := hexutil.Decode(batchCallData.BlockRef)
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

	clauses = make([]*tx.Clause, len(batchCallData.Clauses))
	for i, c := range batchCallData.Clauses {
		var value *big.Int
		if c.Value == nil {
			value = new(big.Int)
		} else {
			value = (*big.Int)(c.Value)
		}
		var data []byte
		if c.Data != "" {
			data, err = hexutil.Decode(c.Data)
			if err != nil {
				err = restutil.BadRequest(errors.WithMessage(err, fmt.Sprintf("data[%d]", i)))
				return
			}
		}
		clauses[i] = tx.NewClause(c.To).WithData(data).WithValue(value)
	}
	return
}

func (a *Accounts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/*").
		Methods(http.MethodPost).
		Name("POST /accounts/*").
		HandlerFunc(restutil.WrapHandlerFunc(a.handleCallBatchCode))
	sub.Path("/{address}").
		Methods(http.MethodGet).
		Name("GET /accounts/{address}").
		HandlerFunc(restutil.WrapHandlerFunc(a.handleGetAccount))
	sub.Path("/{address}/code").
		Methods(http.MethodGet).
		Name("GET /accounts/{address}/code").
		HandlerFunc(restutil.WrapHandlerFunc(a.handleGetCode))
	sub.Path("/{address}/storage/{key}").
		Methods("GET").
		Name("GET /accounts/{address}/storage").
		HandlerFunc(restutil.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("/{address}/storage/raw/{key}").
		Methods("GET").
		Name("GET /accounts/{address}/storage/raw").
		HandlerFunc(restutil.WrapHandlerFunc(a.handleGetRawStorage))

	// These two methods are currently deprecated
	callContractHandler := restutil.HandleGone
	if a.enabledDeprecated {
		callContractHandler = a.handleCallContract
	}
	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /accounts").
		HandlerFunc(restutil.WrapHandlerFunc(callContractHandler))
	sub.Path("/{address}").
		Methods(http.MethodPost).
		Name("POST /accounts/{address}").
		HandlerFunc(restutil.WrapHandlerFunc(callContractHandler))
}
