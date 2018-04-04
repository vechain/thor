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

func (a *Accounts) getAccount(addr thor.Address, stateRoot thor.Bytes32) (*Account, error) {
	state, err := a.stateCreator.NewState(stateRoot)
	if err != nil {
		return nil, err
	}
	b := state.GetBalance(addr)
	code := state.GetCode(addr)
	if err := state.Error(); err != nil {
		return nil, err
	}
	return &Account{
		Balance: math.HexOrDecimal256(*b),
		Code:    hexutil.Encode(code),
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

func (a *Accounts) defaultContractCallOptions() *ContractCallOptions {
	gp := big.NewInt(2106)
	gasPrice := math.HexOrDecimal256(*gp)
	v := big.NewInt(0)
	value := math.HexOrDecimal256(*v)
	return &ContractCallOptions{
		ClauseIndex: 0,
		Gas:         21000,
		From:        &thor.Address{},
		GasPrice:    &gasPrice,
		TxID:        &thor.Bytes32{},
		Value:       &value,
	}
}

func (a *Accounts) santinizeOptions(options *ContractCallOptions) {
	defaultOptions := a.defaultContractCallOptions()

	if options.ClauseIndex < defaultOptions.ClauseIndex {
		options.ClauseIndex = defaultOptions.ClauseIndex
	}
	if options.Gas < defaultOptions.Gas {
		options.Gas = defaultOptions.Gas
	}

	if options.GasPrice == nil {
		options.GasPrice = defaultOptions.GasPrice
	} else {
		gp := big.Int(*options.GasPrice)
		gpDefault := big.Int(*defaultOptions.GasPrice)
		if (&gp).Cmp(&gpDefault) < 0 {
			options.GasPrice = defaultOptions.GasPrice
		}
	}

	if options.Value == nil {
		options.Value = defaultOptions.Value
	}

	if options.From == nil {
		options.From = defaultOptions.From
	}

	if options.TxID == nil {
		options.TxID = defaultOptions.TxID
	}
}

//Call a contract with input
func (a *Accounts) Call(to *thor.Address, body *ContractCallBody, block *block.Block) (output []byte, err error) {
	a.santinizeOptions(&body.Options)
	state, err := a.stateCreator.NewState(block.Header().StateRoot())
	if err != nil {
		return nil, err
	}
	header := block.Header()
	rt := runtime.New(state, header.Beneficiary(), header.Number(), header.Timestamp(), header.GasLimit(), nil)
	v := big.Int(*body.Options.Value)
	data, err := hexutil.Decode(body.Input)
	if err != nil {
		return nil, err
	}
	clause := tx.NewClause(to).WithData(data).WithValue(&v)
	var vmout *vm.Output
	gp := big.Int(*body.Options.GasPrice)
	vmout = rt.Call(clause, body.Options.ClauseIndex, body.Options.Gas, *body.Options.From, &gp, *body.Options.TxID)
	if err := state.Error(); err != nil {
		return nil, err
	}
	if vmout.VMErr != nil {
		return nil, vmout.VMErr
	}
	return vmout.Value, nil

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
	callBody := &ContractCallBody{}
	if err := json.Unmarshal(res, &callBody); err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}

	b, err := a.getBlock(req.URL.Query().Get("revision"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "revision"), http.StatusBadRequest)
	}

	output, err := a.Call(&addr, callBody, b)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, hexutil.Encode(output))
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
	acc, err := a.getAccount(addr, b.Header().StateRoot())
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
	return utils.WriteJSON(w, storage.String())
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
func (a *Accounts) filter(option *logdb.FilterOption) ([]Log, error) {
	logs, err := a.logDB.Filter(option)
	if err != nil {
		return nil, err
	}
	lgs := make([]Log, len(logs))
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
	var f *FilterTopics
	if len(res) != 0 {
		if err := json.Unmarshal(res, &f); err != nil {
			return utils.HTTPError(err, http.StatusBadRequest)
		}
	}
	params := mux.Vars(req)
	addr, err := thor.ParseAddress(params["address"])
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	fromBlock, err := a.parseFromBlock(req.URL.Query().Get("fromBlock"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "fromBlock"), http.StatusBadRequest)
	}
	toBlock, err := a.parseToBlock(req.URL.Query().Get("toBlock"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "toBlock"), http.StatusBadRequest)
	}
	offset, err := a.parseOffset(req.URL.Query().Get("offset"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "offset"), http.StatusBadRequest)
	}
	limit, err := a.parseLimit(req.URL.Query().Get("limit"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "limit"), http.StatusBadRequest)
	}
	options := &logdb.FilterOption{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Address:   &addr,
		TopicSet:  f.TopicSet,
		Offset:    offset,
		Limit:     limit,
	}
	logs, err := a.filter(options)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, logs)
}

func (a *Accounts) parseOffset(offset string) (uint64, error) {
	if offset == "" {
		return math.MaxUint64, nil
	}
	n, err := strconv.ParseUint(offset, 0, 0)
	if err != nil {
		return math.MaxUint64, err
	}
	return uint64(n), nil
}

func (a *Accounts) parseLimit(limit string) (uint32, error) {
	if limit == "" {
		return math.MaxUint32, nil
	}
	n, err := strconv.ParseUint(limit, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

func (a *Accounts) parseFromBlock(blkNum string) (uint32, error) {
	if blkNum == "" {
		return 0, nil
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

func (a *Accounts) parseToBlock(blkNum string) (uint32, error) {
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

	sub.Path("/{address}/storage").Queries("key", "{key}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("/{address}/storage").Queries("key", "{key}", "revision", "{revision}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))

	sub.Path("/{address}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))
	sub.Path("/{address}").Queries("revision", "{revision}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleCallContract))

	sub.Path("/{address}/logs").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(a.handleFilterLogs))
}
