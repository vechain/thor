// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"context"
	"fmt"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

type Accounts struct {
	repo         *chain.Repository
	stater       *state.Stater
	callGasLimit uint64
	forkConfig   thor.ForkConfig
	bft          bft.Committer
}

func New(
	repo *chain.Repository,
	stater *state.Stater,
	callGasLimit uint64,
	forkConfig thor.ForkConfig,
	bft bft.Committer,
) *Accounts {
	return &Accounts{
		repo,
		stater,
		callGasLimit,
		forkConfig,
		bft,
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
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}

	_, st, err := utils.GetSummaryAndState(revision, a.repo, a.bft, a.stater)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	code, err := a.getCode(addr, st)
	if err != nil {
		return err
	}

	return utils.WriteJSON(w, &GetCodeResult{Code: hexutil.Encode(code)})
}

func (a *Accounts) getAccount(addr thor.Address, header *block.Header, state *state.State) (*Account, error) {
	b, err := state.GetBalance(addr)
	if err != nil {
		return nil, err
	}
	code, err := state.GetCode(addr)
	if err != nil {
		return nil, err
	}
	energy, err := state.GetEnergy(addr, header.Timestamp())
	if err != nil {
		return nil, err
	}

	return &Account{
		Balance: math.HexOrDecimal256(*b),
		Energy:  math.HexOrDecimal256(*energy),
		HasCode: len(code) != 0,
	}, nil
}

func (a *Accounts) getStorage(addr thor.Address, key thor.Bytes32, state *state.State) (thor.Bytes32, error) {
	storage, err := state.GetStorage(addr, key)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return storage, nil
}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}

	summary, st, err := utils.GetSummaryAndState(revision, a.repo, a.bft, a.stater)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	acc, err := a.getAccount(addr, summary.Header, st)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, acc)
}

func (a *Accounts) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	key, err := thor.ParseBytes32(mux.Vars(req)["key"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "key"))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}

	_, st, err := utils.GetSummaryAndState(revision, a.repo, a.bft, a.stater)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	storage, err := a.getStorage(addr, key, st)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, &GetStorageResult{Value: storage.String()})
}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	callData := &CallData{}
	if err := utils.ParseJSON(req.Body, &callData); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), true)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, st, err := utils.GetSummaryAndState(revision, a.repo, a.bft, a.stater)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	var addr *thor.Address
	if mux.Vars(req)["address"] != "" {
		address, err := thor.ParseAddress(mux.Vars(req)["address"])
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "address"))
		}
		addr = &address
	}
	var batchCallData = &BatchCallData{
		Clauses: Clauses{
			Clause{
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
	return utils.WriteJSON(w, results[0])
}

func (a *Accounts) handleCallBatchCode(w http.ResponseWriter, req *http.Request) error {
	batchCallData := &BatchCallData{}
	if err := utils.ParseJSON(req.Body, &batchCallData); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	revision, err := utils.ParseRevision(req.URL.Query().Get("revision"), true)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	summary, st, err := utils.GetSummaryAndState(revision, a.repo, a.bft, a.stater)
	if err != nil {
		if a.repo.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	results, err := a.batchCall(req.Context(), batchCallData, summary.Header, st)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, results)
}

func (a *Accounts) batchCall(
	ctx context.Context,
	batchCallData *BatchCallData,
	header *block.Header,
	st *state.State,
) (results BatchCallResults, err error) {
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
		},
		a.forkConfig)
	results = make(BatchCallResults, 0)
	resultCh := make(chan interface{}, 1)
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
				results = append(results, convertCallResultWithInputGas(v, gas))
				if v.VMErr != nil {
					return results, nil
				}
				gas = v.LeftOverGas
			}
		}
	}
	return results, nil
}

func (a *Accounts) handleBatchCallData(batchCallData *BatchCallData) (txCtx *xenv.TransactionContext, gas uint64, clauses []*tx.Clause, err error) {
	if batchCallData.Gas > a.callGasLimit {
		return nil, 0, nil, utils.Forbidden(errors.New("gas: exceeds limit"))
	} else if batchCallData.Gas == 0 {
		gas = a.callGasLimit
	} else {
		gas = batchCallData.Gas
	}

	txCtx = &xenv.TransactionContext{}

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
	txCtx.Expiration = batchCallData.Expiration

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
				err = utils.BadRequest(errors.WithMessage(err, fmt.Sprintf("data[%d]", i)))
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
		Name("accounts_call_batch_code").
		HandlerFunc(utils.WrapHandlerFunc(a.handleCallBatchCode))
	sub.Path("/{address}").
		Methods(http.MethodGet).
		Name("accounts_get_account").
		HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))
	sub.Path("/{address}/code").
		Methods(http.MethodGet).
		Name("accounts_get_code").
		HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))
	sub.Path("/{address}/storage/{key}").
		Methods("GET").
		Name("accounts_get_storage").
		HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	// These two methods are currently deprecated
	sub.Path("").
		Methods(http.MethodPost).
		Name("accounts_call_contract").
		HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("/{address}").
		Methods(http.MethodPost).
		Name("accounts_call_contract_address").
		HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
}
