// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"context"
	"math/big"
	"net/http"
	"strconv"
	"time"

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
}

func New(chain *chain.Chain, stateCreator *state.Creator) *Accounts {
	return &Accounts{
		chain,
		stateCreator,
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
	revision, err := a.parseRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	h, err := a.getBlockHeader(revision)
	if err != nil {
		if a.chain.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
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

func (a *Accounts) sterilizeOptions(options *ContractCall) {
	if options.Gas == 0 {
		options.Gas = math.MaxUint64
	}
	if options.GasPrice == nil {
		gp := new(big.Int)
		dgp := math.HexOrDecimal256(*gp)
		options.GasPrice = &dgp
	}
	if options.Value == nil {
		v := new(big.Int)
		dv := math.HexOrDecimal256(*v)
		options.Value = &dv
	}
}

//Call a contract with input
func (a *Accounts) Call(ctx context.Context, to *thor.Address, body *ContractCall, header *block.Header) (output *VMOutput, err error) {
	a.sterilizeOptions(body)
	state, err := a.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	v := big.Int(*body.Value)
	data, err := hexutil.Decode(body.Data)
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(to).WithData(data).WithValue(&v)
	gp := (*big.Int)(body.GasPrice)
	signer, _ := header.Signer()
	rt := runtime.New(a.chain.NewSeeker(header.ParentID()), state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore()})

	exec, interrupt := rt.PrepareClause(clause, 0, body.Gas, &xenv.TransactionContext{
		Origin:     body.Caller,
		GasPrice:   gp,
		ProvedWork: &big.Int{}})
	vmout := make(chan *runtime.Output, 1)
	go func() {
		o, _ := exec()
		vmout <- o
	}()
	select {
	case <-ctx.Done():
		interrupt()
		return nil, ctx.Err()
	case vo := <-vmout:
		if err := rt.Seeker().Err(); err != nil {
			return nil, err
		}
		if err := state.Err(); err != nil {
			return nil, err
		}
		return convertVMOutputWithInputGas(vo, body.Gas), nil
	}
}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	revision, err := a.parseRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	h, err := a.getBlockHeader(revision)
	if err != nil {
		if a.chain.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
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
	revision, err := a.parseRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	h, err := a.getBlockHeader(revision)
	if err != nil {
		if a.chain.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	storage, err := a.getStorage(addr, key, h.StateRoot())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, map[string]string{"value": storage.String()})
}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	callBody := &ContractCall{}
	if err := utils.ParseJSON(req.Body, &callBody); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	revision, err := a.parseRevision(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	h, err := a.getBlockHeader(revision)
	if err != nil {
		if a.chain.IsNotFound(err) {
			return utils.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}
	address := mux.Vars(req)["address"]
	ctx, cancel := context.WithTimeout(req.Context(), time.Second*10)
	defer cancel()
	if address == "" {
		output, err := a.Call(ctx, nil, callBody, h)
		if err != nil {
			return err
		}
		return utils.WriteJSON(w, output)
	}
	addr, err := thor.ParseAddress(address)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "address"))
	}
	output, err := a.Call(ctx, &addr, callBody, h)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, output)
}

func (a *Accounts) parseRevision(revision string) (interface{}, error) {
	if revision == "" || revision == "best" {
		return nil, nil
	}
	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, err
		}
		return blockID, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, err
	}
	if n > math.MaxUint32 {
		return nil, errors.New("block number out of max uint32")
	}
	return uint32(n), err
}

func (a *Accounts) getBlockHeader(revision interface{}) (*block.Header, error) {
	switch revision.(type) {
	case thor.Bytes32:
		return a.chain.GetBlockHeader(revision.(thor.Bytes32))
	case uint32:
		return a.chain.GetTrunkBlockHeader(revision.(uint32))
	default:
		return a.chain.BestBlock().Header(), nil
	}
}

func (a *Accounts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/{address}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))
	sub.Path("/{address}").Queries("revision", "{revision}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))

	sub.Path("/{address}/code").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))

	sub.Path("/{address}/storage/{key}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("/{address}/storage/{key}").Queries("revision", "{revision}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("").Queries("revision", "{revision}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))

	sub.Path("/{address}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("/{address}").Queries("revision", "{revision}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))

}
