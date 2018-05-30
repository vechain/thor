// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
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
		return utils.BadRequest(err, "address")
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	code, err := a.getCode(addr, b.Header().StateRoot())
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
func (a *Accounts) Call(to *thor.Address, body *ContractCall, header *block.Header) (output *VMOutput, err error) {
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
	proposer, _ := header.Signer()
	rt := runtime.New(a.chain.NewSeeker(header.ParentID()), state,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Proposer:    proposer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore()})

	vmout := rt.Call(clause, 0, body.Gas, &xenv.TransactionContext{
		Origin:     body.Caller,
		GasPrice:   gp,
		ProvedWork: &big.Int{}})

	if err := rt.Seeker().Err(); err != nil {
		return nil, err
	}
	if err := state.Err(); err != nil {
		return nil, err
	}
	return convertVMOutputWithInputGas(vmout, body.Gas), nil

}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(err, "address")
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	acc, err := a.getAccount(addr, b.Header())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, acc)
}

func (a *Accounts) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.BadRequest(err, "address")
	}
	key, err := thor.ParseBytes32(mux.Vars(req)["key"])
	if err != nil {
		return utils.BadRequest(err, "key")
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	storage, err := a.getStorage(addr, key, b.Header().StateRoot())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, map[string]string{"value": storage.String()})
}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	callBody := &ContractCall{}
	if err := utils.ParseJSON(req.Body, &callBody); err != nil {
		return err
	}
	req.Body.Close()
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return err
	}
	address := mux.Vars(req)["address"]
	var output *VMOutput
	if address == "" {
		output, err = a.Call(nil, callBody, b.Header())
	} else {
		addr, err := thor.ParseAddress(address)
		if err != nil {
			return utils.BadRequest(err, "address")
		}
		output, err = a.Call(&addr, callBody, b.Header())
	}
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, output)
}

func (a *Accounts) getBlock(revision string) (*block.Block, error) {
	if revision == "" || revision == "best" {
		return a.chain.BestBlock(), nil
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
		return a.chain.GetTrunkBlock(uint32(n))
	}
	return a.chain.GetBlock(blkID)
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
