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
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
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

func (a *Accounts) getStateRoot(blockNum uint32) (thor.Bytes32, error) {
	var (
		block *block.Block
		err   error
	)

	if blockNum == math.MaxUint32 {
		block, err = a.chain.GetBestBlock()
	} else {
		block, err = a.chain.GetBlockByNumber(blockNum)
	}
	if err != nil {
		return thor.Bytes32{}, err
	}
	return block.Header().StateRoot(), nil
}

func (a *Accounts) getState(blockNum uint32) (*state.State, error) {
	root, err := a.getStateRoot(blockNum)
	if err != nil {
		return nil, err
	}
	return a.stateCreator.NewState(root)
}

func (a *Accounts) getBalance(addr thor.Address, blockNum uint32) (*big.Int, error) {
	state, err := a.getState(blockNum)
	if err != nil {
		return nil, err
	}
	bal := state.GetBalance(addr)
	if err := state.Error(); err != nil {
		return nil, err
	}
	return bal, nil
}

func (a *Accounts) getCode(addr thor.Address, blockNum uint32) ([]byte, error) {
	state, err := a.getState(blockNum)
	if err != nil {
		return nil, err
	}
	code := state.GetCode(addr)
	if err := state.Error(); err != nil {
		return nil, err
	}
	return code, nil
}

func (a *Accounts) getStorage(addr thor.Address, key thor.Bytes32, blockNum uint32) (thor.Bytes32, error) {
	state, err := a.getState(blockNum)
	if err != nil {
		return thor.Bytes32{}, err
	}
	storage := state.GetStorage(addr, key)
	if err := state.Error(); err != nil {
		return thor.Bytes32{}, err
	}
	return storage, nil
}

func (a *Accounts) handleGetBalance(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	blockNum, err := a.parseBlockNum(req.URL.Query().Get("blockNumber"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "blockNumber"), http.StatusBadRequest)
	}
	balance, err := a.getBalance(addr, blockNum)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, (*math.HexOrDecimal256)(balance))
}

func (a *Accounts) handleGetCode(w http.ResponseWriter, req *http.Request) error {
	hexAddr := mux.Vars(req)["address"]
	addr, err := thor.ParseAddress(hexAddr)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "address"), http.StatusBadRequest)
	}
	blockNum, err := a.parseBlockNum(req.URL.Query().Get("blockNumber"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "blockNumber"), http.StatusBadRequest)
	}
	code, err := a.getCode(addr, blockNum)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, hexutil.Encode(code))
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
	blockNum, err := a.parseBlockNum(req.URL.Query().Get("blockNumber"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "blockNumber"), http.StatusBadRequest)
	}

	storage, err := a.getStorage(addr, key, blockNum)
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

func (a *Accounts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/{address}/balance").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetBalance))
	sub.Path("/{address}/balance").Queries("blockNumber", "{blockNumber}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetBalance))

	sub.Path("/{address}/code").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))
	sub.Path("/{address}/code").Queries("blockNumber", "{blockNumber}").Methods(http.MethodGet).HandlerFunc(utils.WrapHandlerFunc(a.handleGetCode))

	sub.Path("/{address}/storage").Queries("key", "{key}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
	sub.Path("/{address}/storage").Queries("key", "{key}", "blockNumber", "{blockNumber}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(a.handleGetStorage))
}
