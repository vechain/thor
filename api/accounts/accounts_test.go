// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	ABI "github.com/vechain/thor/abi"
	"github.com/vechain/thor/api/accounts"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var sol = `	pragma solidity ^0.4.18;
			contract Test {
    			uint8 value;
    			function add(uint8 a,uint8 b) public pure returns(uint8) {
        			return a+b;
    			}
    			function set(uint8 v) public {
        			value = v;
    			}
			}`

var abiJSON = `[
	{
		"constant": false,
		"inputs": [
			{
				"name": "v",
				"type": "uint8"
			}
		],
		"name": "set",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
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
var addr = thor.BytesToAddress([]byte("to"))
var value = big.NewInt(10000)
var storageKey = thor.Bytes32{}
var storageValue = byte(1)

var contractAddr thor.Address

var bytecode = common.Hex2Bytes("608060405234801561001057600080fd5b50610125806100206000396000f3006080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")

var runtimeBytecode = common.Hex2Bytes("6080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")

var invalidAddr = "abc"                                                                   //invlaid address
var invalidBytes32 = "0x000000000000000000000000000000000000000000000000000000000000000g" //invlaid bytes32
var invalidNumberRevision = "4294967296"                                                  //invalid block number

var ts *httptest.Server

func TestAccount(t *testing.T) {
	initAccountServer(t)
	defer ts.Close()
	getAccount(t)
	getCode(t)
	getStorage(t)
	deployContractWithCall(t)
	callContract(t)
	batchCall(t)
}

func getAccount(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	res, statusCode = httpGet(t, ts.URL+"/accounts/"+addr.String()+"?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode = httpGet(t, ts.URL+"/accounts/"+addr.String())
	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, math.HexOrDecimal256(*value), acc.Balance, "balance should be equal")
	assert.Equal(t, http.StatusOK, statusCode, "OK")

}

func getCode(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr+"/code")
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	res, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/code?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/code")
	var code map[string]string
	if err := json.Unmarshal(res, &code); err != nil {
		t.Fatal(err)
	}
	c, err := hexutil.Decode(code["code"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, runtimeBytecode, c, "code should be equal")
	assert.Equal(t, http.StatusOK, statusCode, "OK")
}

func getStorage(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr+"/storage/"+storageKey.String())
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	res, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+invalidBytes32)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad storage key")

	res, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+storageKey.String()+"?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+storageKey.String())
	var value map[string]string
	if err := json.Unmarshal(res, &value); err != nil {
		t.Fatal(err)
	}
	h, err := thor.ParseBytes32(value["value"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, thor.BytesToBytes32([]byte{storageValue}), h, "storage should be equal")
	assert.Equal(t, http.StatusOK, statusCode, "OK")
}

func initAccountServer(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)
	claTransfer := tx.NewClause(&addr).WithValue(value)
	claDeploy := tx.NewClause(nil).WithData(bytecode)
	transaction := buildTxWithClauses(t, repo.ChainTag(), claTransfer, claDeploy)
	contractAddr = thor.CreateContractAddress(transaction.ID(), 1, 0)
	packTx(repo, stater, transaction, t)

	method := "set"
	abi, err := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(uint8(storageValue))
	if err != nil {
		t.Fatal(err)
	}
	claCall := tx.NewClause(&contractAddr).WithData(input)
	transactionCall := buildTxWithClauses(t, repo.ChainTag(), claCall)
	packTx(repo, stater, transactionCall, t)

	router := mux.NewRouter()
	accounts.New(repo, stater, math.MaxUint64, thor.NoFork).Mount(router, "/accounts")
	ts = httptest.NewServer(router)
}

func buildTxWithClauses(t *testing.T, chaiTag byte, clauses ...*tx.Clause) *tx.Transaction {
	builder := new(tx.Builder).
		ChainTag(chaiTag).
		Expiration(10).
		Gas(1000000)
	for _, c := range clauses {
		builder.Clause(c)
	}

	transaction := builder.Build()
	sig, err := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	return transaction.WithSignature(sig)
}

func packTx(repo *chain.Repository, stater *state.Stater, transaction *tx.Transaction, t *testing.T) {
	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	flow, err := packer.Schedule(repo.BestBlockSummary(), uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(transaction)
	if err != nil {
		t.Fatal(err)
	}
	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBlock(b, receipts, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBestBlockID(b.Header().ID()); err != nil {
		t.Fatal(err)
	}
}

func deployContractWithCall(t *testing.T) {
	badBody := &accounts.CallData{
		Gas:  10000000,
		Data: "abc",
	}
	res, statusCode := httpPost(t, ts.URL+"/accounts", badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad data")

	reqBody := &accounts.CallData{
		Gas:  10000000,
		Data: hexutil.Encode(bytecode),
	}

	res, statusCode = httpPost(t, ts.URL+"/accounts?revision="+invalidNumberRevision, reqBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode = httpPost(t, ts.URL+"/accounts", reqBody)
	var output *accounts.CallResult
	if err := json.Unmarshal(res, &output); err != nil {
		t.Fatal(err)
	}
	assert.False(t, output.Reverted)

}

func callContract(t *testing.T) {
	res, statusCode := httpPost(t, ts.URL+"/accounts/"+invalidAddr, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	badBody := &accounts.CallData{
		Data: "input",
	}
	res, statusCode = httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid input data")

	a := uint8(1)
	b := uint8(2)
	method := "add"
	abi, err := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := &accounts.CallData{
		Data: hexutil.Encode(input),
	}
	res, statusCode = httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), reqBody)
	var output *accounts.CallResult
	if err = json.Unmarshal(res, &output); err != nil {
		t.Fatal(err)
	}
	data, err := hexutil.Decode(output.Data)
	if err != nil {
		t.Fatal(err)
	}
	var ret uint8
	err = m.DecodeOutput(data, &ret)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, a+b, ret)
}

func batchCall(t *testing.T) {
	badBody := &accounts.BatchCallData{
		Clauses: accounts.Clauses{
			accounts.Clause{
				To:    &contractAddr,
				Data:  "data1",
				Value: nil,
			},
			accounts.Clause{
				To:    &contractAddr,
				Data:  "data2",
				Value: nil,
			}},
	}
	res, statusCode := httpPost(t, ts.URL+"/accounts/*", badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid data")

	a := uint8(1)
	b := uint8(2)
	method := "add"
	abi, err := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := &accounts.BatchCallData{
		Clauses: accounts.Clauses{
			accounts.Clause{
				To:    &contractAddr,
				Data:  hexutil.Encode(input),
				Value: nil,
			},
			accounts.Clause{
				To:    &contractAddr,
				Data:  hexutil.Encode(input),
				Value: nil,
			}},
	}

	res, statusCode = httpPost(t, ts.URL+"/accounts/*?revision="+invalidNumberRevision, badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid revision")

	res, statusCode = httpPost(t, ts.URL+"/accounts/*", reqBody)
	var results accounts.BatchCallResults
	if err = json.Unmarshal(res, &results); err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		data, err := hexutil.Decode(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		var ret uint8
		err = m.DecodeOutput(data, &ret)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, a+b, ret, "should be equal")
	}
	assert.Equal(t, http.StatusOK, statusCode)

	big := math.HexOrDecimal256(*big.NewInt(1000))
	fullBody := &accounts.BatchCallData{
		Clauses:    accounts.Clauses{},
		Gas:        21000,
		GasPrice:   &big,
		ProvedWork: &big,
		Caller:     &contractAddr,
		GasPayer:   &contractAddr,
		Expiration: 100,
		BlockRef:   "0x00000000aabbccdd",
	}
	_, statusCode = httpPost(t, ts.URL+"/accounts/*", fullBody)
	assert.Equal(t, http.StatusOK, statusCode)
}

func httpPost(t *testing.T, url string, body interface{}) ([]byte, int) {
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}
