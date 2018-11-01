// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

type Accounts struct {
	chain        *chain.Chain
	stateCreator *state.Creator
	callGasLimit uint64
}

func New(chain *chain.Chain, stateCreator *state.Creator, callGasLimit uint64) *Accounts {
	return &Accounts{
		chain,
		stateCreator,
		callGasLimit,
	}
}

func (a *Accounts) getCode(addr thor.Address, stateRoot thor.Bytes32) ([]byte, error) {
	state, err := a.stateCreator.NewState(stateRoot)
	if err != nil {
		return nil, err
	}
	code := state.GetCode(addr)
	if err := state.Err(); err != nil {
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
	h, err := a.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	code, err := a.getCode(addr, h.StateRoot())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, map[string]string{"code": hexutil.Encode(code)})
}

func (a *Accounts) getAccount(addr thor.Address, header *block.Header) (*Account, error) {
	state, err := a.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	b := state.GetBalance(addr)
	code := state.GetCode(addr)
	energy := state.GetEnergy(addr, header.Timestamp())
	if err := state.Err(); err != nil {
		return nil, err
	}
	return &Account{
		Balance: math.HexOrDecimal256(*b),
		Energy:  math.HexOrDecimal256(*energy),
		HasCode: len(code) != 0,
	}, nil
}

func (a *Accounts) getStorage(addr thor.Address, key thor.Bytes32, stateRoot thor.Bytes32) (thor.Bytes32, error) {
	state, err := a.stateCreator.NewState(stateRoot)
	if err != nil {
		return thor.Bytes32{}, err
	}
	storage := state.GetStorage(addr, key)
	if err := state.Err(); err != nil {
		return thor.Bytes32{}, err
	}
	return storage, nil
}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	h, err := a.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	acc, err := a.getAccount(addr, h)
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
	h, err := a.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	storage, err := a.getStorage(addr, key, h.StateRoot())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, map[string]string{"value": storage.String()})
}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	callData := &CallData{}
	if err := utils.ParseJSON(req.Body, &callData); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	h, err := a.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
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
	results, err := a.batchCall(req.Context(), batchCallData, h)
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
	h, err := a.handleRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	results, err := a.batchCall(req.Context(), batchCallData, h)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, results)
}

func (a *Accounts) batchCall(ctx context.Context, batchCallData *BatchCallData, header *block.Header) (results BatchCallResults, err error) {
	gas, gasPrice, caller, clauses, err := a.handleBatchCallData(batchCallData)
	if err != nil {
		return nil, err
	}
	state, err := a.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	signer, _ := header.Signer()
	rt := runtime.New(a.chain.NewSeeker(header.ParentID()), state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore()})
	results = make(BatchCallResults, 0)
	vmout := make(chan *runtime.Output, 1)
	for i, clause := range clauses {
		exec, interrupt := rt.PrepareClause(clause, uint32(i), gas, &xenv.TransactionContext{
			Origin:     *caller,
			GasPrice:   gasPrice,
			ProvedWork: &big.Int{}})
		go func() {
			out, _ := exec()
			vmout <- out
		}()
		select {
		case <-ctx.Done():
			interrupt()
			return nil, ctx.Err()
		case out := <-vmout:
			if err := rt.Seeker().Err(); err != nil {
				return nil, err
			}
			if err := state.Err(); err != nil {
				return nil, err
			}
			results = append(results, convertCallResultWithInputGas(out, gas))
			if out.VMErr != nil {
				return results, nil
			}
			gas = out.LeftOverGas
		}
	}
	return results, nil
}

func (a *Accounts) handleBatchCallData(batchCallData *BatchCallData) (gas uint64, gasPrice *big.Int, caller *thor.Address, clauses []*tx.Clause, err error) {
	if batchCallData.Gas > a.callGasLimit {
		return 0, nil, nil, nil, utils.Forbidden(errors.New("gas: exceeds limit"))
	} else if batchCallData.Gas == 0 {
		gas = a.callGasLimit
	} else {
		gas = batchCallData.Gas
	}
	if batchCallData.GasPrice == nil {
		gasPrice = new(big.Int)
	} else {
		gasPrice = (*big.Int)(batchCallData.GasPrice)
	}
	if batchCallData.Caller == nil {
		caller = &thor.Address{}
	} else {
		caller = batchCallData.Caller
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

func (a *Accounts) handleRevision(revision string) (*block.Header, error) {
	if revision == "" || revision == "best" {
		return a.chain.BestBlock().Header(), nil
	}
	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		h, err := a.chain.GetBlockHeader(blockID)
		if err != nil {
			if a.chain.IsNotFound(err) {
				return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
			}
			return nil, err
		}
		return h, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	if n > math.MaxUint32 {
		return nil, utils.BadRequest(errors.WithMessage(errors.New("block number out of max uint32"), "revision"))
	}
	h, err := a.chain.GetTrunkBlockHeader(uint32(n))
	if err != nil {
		if a.chain.IsNotFound(err) {
			return nil, utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return nil, err
	}
	return h, nil
}

func (a *Accounts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/*").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallBatchCode))
	sub.Path("/{address}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))
	sub.Path("/{address}/code").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))
	sub.Path("/{address}/storage/{key}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("/{address}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))

}
