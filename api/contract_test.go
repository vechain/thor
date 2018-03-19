package api_test

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	ABI "github.com/vechain/thor/builtin/abi"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var sol = `	pragma solidity ^0.4.18;
						contract Test {

    						function add(uint8 a,uint8 b) public pure returns(uint8) {
        						return a+b;
								}

						}`

var abiJSON = `[
	{
		"constant": true,
		"inputs": [
			{
				"name": "a",
				"type": "uint8"
			},
			{
				"name": "b",
				"type": "uint8"
			}
		],
		"name": "add",
		"outputs": [
			{
				"name": "",
				"type": "uint8"
			}
		],
		"payable": false,
		"stateMutability": "pure",
		"type": "function"
	}
]`
var contractAddr = thor.BytesToAddress([]byte("contract"))
var code = common.Hex2Bytes("606060405260043610603f576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063bb4e3f4d146044575b600080fd5b3415604e57600080fd5b6071600480803560ff1690602001909190803560ff16906020019091905050608d565b604051808260ff1660ff16815260200191505060405180910390f35b60008183019050929150505600a165627a7a72305820080cbeb07e393a5e37a16fff2145c3344b9cf35a9e3202e68036015e968ff14f0029")

func TestContract(t *testing.T) {
	ts, ci := initServer(t)
	callContract(t, ts, ci, contractAddr)
}

func initServer(t *testing.T) (*httptest.Server, *api.ContractInterface) {
	db, err := lvldb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	chain := chain.New(db)
	stateC := state.NewCreator(db)
	b, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain.WriteGenesis(b)
	best, _ := chain.GetBestBlock()
	st, _ := stateC.NewState(best.Header().StateRoot())
	st.SetCode(contractAddr, code)
	hash, _ := st.Stage().Commit()
	blk := new(block.Builder).ParentID(b.Header().ID()).StateRoot(hash).Build()
	_, err = chain.AddBlock(blk, true)
	if err != nil {
		t.Fatal(err)
	}
	ci := api.NewContractInterface(chain, stateC)
	router := mux.NewRouter()
	api.NewContractHTTPRouter(router, ci)
	ts := httptest.NewServer(router)

	return ts, ci
}

func callContract(t *testing.T, ts *httptest.Server, ci *api.ContractInterface, contractAddr thor.Address) {
	a := uint8(1)
	b := uint8(2)

	method := "add"
	abi, err := ABI.New(strings.NewReader(abiJSON))
	codec, err := abi.ForMethod(method)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, method, codec.Name())
	input, err := codec.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	options := ci.DefaultContractInterfaceOptions()
	optionsData, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}

	r, err := httpPostForm(ts, ts.URL+"/contracts/"+contractAddr.String(), url.Values{"input": {string(input)}, "options": {string(optionsData)}})
	if err != nil {
		t.Fatal(err)
	}
	var v uint8
	err = codec.DecodeOutput(r, &v)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, a+b, v, "should be equal")

}
