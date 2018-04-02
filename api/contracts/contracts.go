package contracts

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
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

//ContractCallBody represents contract-call body
type ContractCallBody struct {
	Input   string              `json:"input"`
	Options ContractCallOptions `json:"options"`
}

// ContractCallOptions represents options in contract-call body
type ContractCallOptions struct {
	ClauseIndex uint32                `json:"clauseIndex"`
	Gas         uint64                `json:"gas,string"`
	From        *thor.Address         `json:"from"`
	GasPrice    *math.HexOrDecimal256 `json:"gasPrice,string"`
	TxID        *thor.Hash            `json:"txID"`
	Value       *math.HexOrDecimal256 `json:"value,string"`
}

//Contracts for call a contract
type Contracts struct {
	chain        *chain.Chain
	stateCreator *state.Creator
}

//New return a BlockMananger by chain
func New(chain *chain.Chain, stateCreator *state.Creator) *Contracts {
	return &Contracts{
		chain,
		stateCreator,
	}
}

func (c *Contracts) defaultContractCallOptions() *ContractCallOptions {
	gp := big.NewInt(1)
	gph := math.HexOrDecimal256(*gp)
	v := big.NewInt(0)
	vh := math.HexOrDecimal256(*v)
	return &ContractCallOptions{
		ClauseIndex: 0,
		Gas:         21000,
		From:        &thor.Address{},
		GasPrice:    &gph,
		TxID:        &thor.Hash{},
		Value:       &vh,
	}
}

func (c *Contracts) santinizeOptions(options *ContractCallOptions) {
	defaultOptions := c.defaultContractCallOptions()

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

func (c *Contracts) getBlock(blockNum uint32) (*block.Block, error) {
	var (
		block *block.Block
		err   error
	)
	if blockNum == math.MaxUint32 {
		block, err = c.chain.GetBestBlock()
	} else {
		block, err = c.chain.GetBlockByNumber(blockNum)
	}
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (c *Contracts) getStateRoot(blockNum uint32) (thor.Hash, error) {
	block, err := c.getBlock(blockNum)
	if err != nil {
		return thor.Hash{}, err
	}
	return block.Header().StateRoot(), nil
}

func (c *Contracts) getState(blockNum uint32) (*state.State, error) {
	root, err := c.getStateRoot(blockNum)
	if err != nil {
		return nil, err
	}
	return c.stateCreator.NewState(root)
}

//Call a contract with input
func (c *Contracts) Call(to *thor.Address, body *ContractCallBody, blockNum uint32) (output []byte, err error) {
	c.santinizeOptions(&body.Options)
	state, err := c.getState(blockNum)
	if err != nil {
		return nil, err
	}
	block, err := c.getBlock(blockNum)
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

func (c *Contracts) handleCallContract(w http.ResponseWriter, req *http.Request) error {
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

	blockNum, err := c.parseBlockNum(req.URL.Query().Get("blockNumber"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "blockNumber"), http.StatusBadRequest)
	}

	output, err := c.Call(&addr, callBody, blockNum)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, hexutil.Encode(output))
}

func (c *Contracts) parseBlockNum(blkNum string) (uint32, error) {
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

func (c *Contracts) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/{address}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(c.handleCallContract))
	sub.Path("/{address}").Queries("blockNumber", "{blockNumber}").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(c.handleCallContract))
}
