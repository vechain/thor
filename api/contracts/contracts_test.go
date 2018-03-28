package contracts_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/contracts"
	"github.com/vechain/thor/block"
	ABI "github.com/vechain/thor/builtin/abi"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
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

func initServer(t *testing.T) (*httptest.Server, *contracts.Contracts) {
	db, err := lvldb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	chain := chain.New(db)
	stateC := state.NewCreator(db)
	b, _, err := genesis.Dev.Build(stateC)
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
	router := mux.NewRouter()
	c := contracts.New(chain, stateC)
	c.Mount(router, "/contracts")
	ts := httptest.NewServer(router)

	return ts, c
}

func callContract(t *testing.T, ts *httptest.Server, c *contracts.Contracts, contractAddr thor.Address) {
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
<<<<<<< HEAD:api/contracts/contracts_test.go

	callBody := &contracts.ContractCallBody{
		Input: hexutil.Encode(input),
	}
	body, err := json.Marshal(callBody)
	if err != nil {
		t.Fatal(err)
	}
	r, err := httpPost(ts, ts.URL+"/contracts/"+contractAddr.String(), body)
	if err != nil {
		t.Fatal(err)
	}
	var res string
=======
	gp := big.NewInt(1)
	gph := math.HexOrDecimal256(*gp)
	v := big.NewInt(0)
	vh := math.HexOrDecimal256(*v)
	reqBody := &api.ContractCallBody{
		Input: hexutil.Encode(input),
		Options: api.ContractCallOptions{
			ClauseIndex: 0,
			Gas:         21000,
			From:        thor.Address{}.String(),
			GasPrice:    &gph,
			TxID:        thor.Hash{}.String(),
			Value:       &vh,
		},
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.Post(ts.URL+"/contracts/"+contractAddr.String(), "application/json", bytes.NewReader(reqBodyBytes))
	if err != nil {
		t.Fatal(err)
	}

	r, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	var res map[string]string
>>>>>>> a730f307b1f5b327c1cad70dd85efc269368a393:api/contracts/contracts_test.go
	if err = json.Unmarshal(r, &res); err != nil {
		t.Fatal(err)
	}
	output, err := hexutil.Decode(res)
	if err != nil {
		t.Fatal(err)
	}
	var ret uint8
	err = codec.DecodeOutput(output, &ret)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, a+b, ret, "should be equal")
}

func httpPost(ts *httptest.Server, url string, data []byte) ([]byte, error) {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func httpPost(ts *httptest.Server, url string, data []byte) ([]byte, error) {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	return r, nil
}
