package accounts

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

type Accounts struct {
	chain        *chain.Chain
	stateCreator *state.Creator
	logDB        *logdb.LogDB
}

func New(chain *chain.Chain, stateCreator *state.Creator, logDB *logdb.LogDB) *Accounts {
	return &Accounts{
		chain,
		stateCreator,
		logDB,
	}
}

func (a *Accounts) getCode(addr thor.Address, stateRoot thor.Bytes32) ([]byte, error) {
	state, err := a.stateCreator.NewState(stateRoot)
	if err != nil {
		return nil, err
	}
	code := state.GetCode(addr)
	if err := state.Error(); err != nil {
		return nil, err
	}
	return code, nil
}

func (a *Accounts) handleGetCode(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "revision"), http.StatusBadRequest)
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
	if err := state.Error(); err != nil {
		return nil, err
	}
	hasCode := false
	if code != nil {
		hasCode = true
	}
	energy := builtin.Energy.WithState(state).GetBalance(header.Timestamp(), addr)
	return &Account{
		Balance: math.HexOrDecimal256(*b),
		Energy:  math.HexOrDecimal256(*energy),
		HasCode: hasCode,
	}, nil
}

func (a *Accounts) getStorage(addr thor.Address, key thor.Bytes32, stateRoot thor.Bytes32) (thor.Bytes32, error) {
	state, err := a.stateCreator.NewState(stateRoot)
	if err != nil {
		return thor.Bytes32{}, err
	}
	storage := state.GetStorage(addr, key)
	if err := state.Error(); err != nil {
		return thor.Bytes32{}, err
	}
	return storage, nil
}

func (a *Accounts) santinizeOptions(options *ContractCall) {
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
	a.santinizeOptions(body)
	state, err := a.stateCreator.NewState(header.StateRoot())
	if err != nil {
		return nil, err
	}
	rt := runtime.New(state, header.Beneficiary(), header.Number(), header.Timestamp(), header.GasLimit(), nil)
	v := big.Int(*body.Value)
	data, err := hexutil.Decode(body.Data)
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(to).WithData(data).WithValue(&v)
	var vmout *vm.Output
	gp := big.Int(*body.GasPrice)
	vmout = rt.Call(clause, 0, body.Gas, body.Caller, &gp, thor.Bytes32{})
	if err := state.Error(); err != nil {
		return nil, err
	}
	return convertVMOutputWithInputGas(vmout, body.Gas), nil

}

func (a *Accounts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	addr, err := thor.ParseAddress(mux.Vars(req)["address"])
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}

	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	req.Body.Close()
	callBody := &ContractCall{}
	if err := json.Unmarshal(res, &callBody); err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}

	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "revision"), http.StatusBadRequest)
	}

	output, err := a.Call(&addr, callBody, b.Header())
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, output)
}

func (a *Accounts) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "revision"), http.StatusBadRequest)
	}
	acc, err := a.getAccount(addr, b.Header())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, acc)
}

func (a *Accounts) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	key, err := thor.ParseBytes32(req.URL.Query().Get("key"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "key"), http.StatusBadRequest)
	}
	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "revision"), http.StatusBadRequest)
	}

	storage, err := a.getStorage(addr, key, b.Header().StateRoot())
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, map[string]string{"value": storage.String()})
}

func (a *Accounts) parseBlockNum(blkNum string) (uint32, error) {
	if blkNum == "" {
		return math.MaxUint32, nil
	}
	n, err := strconv.ParseUint(blkNum, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

//Filter query logs with option
func (a *Accounts) filter(logFilter *LogFilter) ([]FilteredLog, error) {
	lf := convertLogFilter(logFilter)
	logs, err := a.logDB.Filter(lf)
	if err != nil {
		return nil, err
	}
	lgs := make([]FilteredLog, len(logs))
	for i, log := range logs {
		lgs[i] = convertLog(log)
	}
	return lgs, nil
}

func (a *Accounts) handleFilterLogs(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	req.Body.Close()
	logFilter := new(LogFilter)
	if len(res) != 0 {
		if err := json.Unmarshal(res, &logFilter); err != nil {
			return utils.HTTPError(err, http.StatusBadRequest)
		}
	}
	params := mux.Vars(req)
	addr, err := thor.ParseAddress(params["address"])
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	logFilter.Address = &addr
	logs, err := a.filter(logFilter)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, logs)
}

func (a *Accounts) getBlock(revision string) (*block.Block, error) {
	if revision == "" || revision == "best" {
		return a.chain.GetBestBlock()
	}
	blkID, err := thor.ParseBytes32(revision)
	if err != nil {
		n, err := strconv.ParseUint(revision, 0, 0)
		if err != nil {
			return nil, err
		}
		if n > math.MaxUint32 {
			return nil, errors.New("block number exceeded")
		}
		return a.chain.GetBlockByNumber(uint32(n))
	}
	return a.chain.GetBlock(blkID)
}

func (a *Accounts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/{address}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))
	sub.Path("/{address}").Queries("revision", "{revision}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetAccount))

	sub.Path("/{address}/code").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))

	sub.Path("/{address}/storage").Queries("key", "{key}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("/{address}/storage").Queries("key", "{key}", "revision", "{revision}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))

	sub.Path("/{address}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("/{address}").Queries("revision", "{revision}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))

	sub.Path("/{address}/logs").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleFilterLogs))
}
