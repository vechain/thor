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
	"github.com/vechain/thor/lvldb"
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

var ts *httptest.Server

func TestAccount(t *testing.T) {
	initAccountServer(t)
	defer ts.Close()
	getAccount(t)
	deployContractWithCall(t)
	callContract(t)
}

func getAccount(t *testing.T) {
	res := httpGet(t, ts.URL+"/accounts/"+addr.String())
	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, math.HexOrDecimal256(*value), acc.Balance, "balance should be equal")

	res = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/code")
	var code map[string]string
	if err := json.Unmarshal(res, &code); err != nil {
		t.Fatal(err)
	}
	c, err := hexutil.Decode(code["code"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, runtimeBytecode, c, "code should be equal")

	res = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+storageKey.String())
	var value map[string]string
	if err := json.Unmarshal(res, &value); err != nil {
		t.Fatal(err)
	}
	h, err := thor.ParseBytes32(value["value"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, thor.BytesToBytes32([]byte{storageValue}), h, "storage should be equal")

}

func initAccountServer(t *testing.T) {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gene := genesis.NewDevnet()

	b, _, err := gene.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	claTransfer := tx.NewClause(&addr).WithValue(value)
	claDeploy := tx.NewClause(nil).WithData(bytecode)
	transaction := buildTxWithClauses(t, chain.Tag(), claTransfer, claDeploy)
	contractAddr = thor.CreateContractAddress(transaction.ID(), 1, 0)
	packTx(chain, stateC, transaction, t)

	method := "set"
	abi, err := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(uint8(storageValue))
	if err != nil {
		t.Fatal(err)
	}
	claCall := tx.NewClause(&contractAddr).WithData(input)
	transactionCall := buildTxWithClauses(t, chain.Tag(), claCall)
	packTx(chain, stateC, transactionCall, t)

	router := mux.NewRouter()
	accounts.New(chain, stateC).Mount(router, "/accounts")
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

func packTx(chain *chain.Chain, stateC *state.Creator, transaction *tx.Transaction, t *testing.T) {
	b := chain.BestBlock()
	packer := packer.New(chain, stateC, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address)
	flow, err := packer.Schedule(b.Header(), uint64(time.Now().Unix()))
	err = flow.Adopt(transaction)
	if err != nil {
		t.Fatal(err)
	}
	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := chain.AddBlock(b, receipts); err != nil {
		t.Fatal(err)
	}
}

func deployContractWithCall(t *testing.T) {
	reqBody := &accounts.ContractCall{
		Gas:    10000000,
		Caller: thor.Address{},
		Data:   hexutil.Encode(bytecode),
	}
	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	response := httpPost(t, ts.URL+"/accounts", reqBodyBytes)
	var output *accounts.VMOutput
	if err = json.Unmarshal(response, &output); err != nil {
		t.Fatal(err)
	}
	assert.False(t, output.Reverted)

}

func callContract(t *testing.T) {
	a := uint8(1)
	b := uint8(2)

	method := "add"
	abi, err := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := &accounts.ContractCall{
		Data: hexutil.Encode(input),
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	response := httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), reqBodyBytes)
	var output *accounts.VMOutput
	if err = json.Unmarshal(response, &output); err != nil {
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
	assert.Equal(t, a+b, ret, "should be equal")
}

func httpPost(t *testing.T, url string, data []byte) []byte {
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func httpGet(t *testing.T, url string) []byte {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
