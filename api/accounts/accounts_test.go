// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	ABI "github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// pragma solidity ^0.4.18;
// contract Test {
// 	uint8 value;
// 	function add(uint8 a,uint8 b) public pure returns(uint8) {
// 		return a+b;
// 	}
// 	function set(uint8 v) public {
// 		value = v;
// 	}
// }

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
var gasLimit uint64
var genesisBlock *block.Block

var contractAddr thor.Address

var bytecode = common.Hex2Bytes("608060405234801561001057600080fd5b50610125806100206000396000f3006080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")

var runtimeBytecode = common.Hex2Bytes("6080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")

var invalidAddr = "abc"                                                                   //invlaid address
var invalidBytes32 = "0x000000000000000000000000000000000000000000000000000000000000000g" //invlaid bytes32
var invalidNumberRevision = "4294967296"                                                  //invalid block number

var acc *accounts.Accounts
var ts *httptest.Server

func TestAccount(t *testing.T) {
	initAccountServer(t)
	defer ts.Close()
	getAccount(t)
	getAccountWithNonExisitingRevision(t)
	getAccountWithGenesisRevision(t)
	getAccountWithFinalizedRevision(t)
	getCode(t)
	getStorage(t)
	deployContractWithCall(t)
	callContract(t)
	batchCall(t)
}

func getAccount(t *testing.T) {
	_, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode = httpGet(t, ts.URL+"/accounts/"+addr.String()+"?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+addr.String())
	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, math.HexOrDecimal256(*value), acc.Balance, "balance should be equal")
	assert.Equal(t, http.StatusOK, statusCode, "OK")
}

func getAccountWithNonExisitingRevision(t *testing.T) {
	revision64Len := "0123456789012345678901234567890123456789012345678901234567890123"

	_, statusCode := httpGet(t, ts.URL+"/accounts/"+addr.String()+"?revision="+revision64Len)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
}

func getAccountWithGenesisRevision(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+addr.String()+"?revision="+genesisBlock.Header().ID().String())
	assert.Equal(t, http.StatusOK, statusCode, "bad revision")

	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}

	balance, err := acc.Balance.MarshalText()
	assert.NoError(t, err)
	assert.Equal(t, "0x0", string(balance), "balance should be 0")

	energy, err := acc.Energy.MarshalText()
	assert.NoError(t, err)
	assert.Equal(t, "0x0", string(energy), "energy should be 0")

	assert.Equal(t, false, acc.HasCode, "hasCode should be false")
}

func getAccountWithFinalizedRevision(t *testing.T) {
	soloAddress := "0xf077b491b355E64048cE21E3A6Fc4751eEeA77fa"

	genesisAccount := httpGetAccount(t, soloAddress+"?revision="+genesisBlock.Header().ID().String())
	finalizedAccount := httpGetAccount(t, soloAddress+"?revision=finalized")

	genesisEnergy := (*big.Int)(&genesisAccount.Energy)
	finalizedEnergy := (*big.Int)(&finalizedAccount.Energy)

	assert.Equal(t, finalizedEnergy.Cmp(genesisEnergy), 1, "finalized energy should be greater than genesis energy")
}

func getCode(t *testing.T) {
	_, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr+"/code")
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/code?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/code")
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
	_, statusCode := httpGet(t, ts.URL+"/accounts/"+invalidAddr+"/storage/"+storageKey.String())
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+invalidBytes32)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad storage key")

	_, statusCode = httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+storageKey.String()+"?revision="+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+contractAddr.String()+"/storage/"+storageKey.String())
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
	genesisBlock = b
	repo, _ := chain.NewRepository(db, b)
	claTransfer := tx.NewClause(&addr).WithValue(value)
	claDeploy := tx.NewClause(nil).WithData(bytecode)
	transaction := buildTxWithClauses(t, repo.ChainTag(), claTransfer, claDeploy)
	contractAddr = thor.CreateContractAddress(transaction.ID(), 1, 0)
	packTx(repo, stater, transaction, t)

	method := "set"
	abi, _ := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(storageValue)
	if err != nil {
		t.Fatal(err)
	}
	claCall := tx.NewClause(&contractAddr).WithData(input)
	transactionCall := buildTxWithClauses(t, repo.ChainTag(), claCall)
	packTx(repo, stater, transactionCall, t)

	router := mux.NewRouter()
	gasLimit = math.MaxUint32
	revisionHandler := utils.NewRevisionHandler(repo, solo.NewBFTEngine(repo))
	acc = accounts.New(repo, stater, gasLimit, thor.NoFork, revisionHandler)
	acc.Mount(router, "/accounts")
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
	_, statusCode := httpPost(t, ts.URL+"/accounts", badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad data")

	reqBody := &accounts.CallData{
		Gas:  10000000,
		Data: hexutil.Encode(bytecode),
	}

	_, statusCode = httpPost(t, ts.URL+"/accounts?revision="+invalidNumberRevision, reqBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, _ := httpPost(t, ts.URL+"/accounts", reqBody)
	var output *accounts.CallResult
	if err := json.Unmarshal(res, &output); err != nil {
		t.Fatal(err)
	}
	assert.False(t, output.Reverted)
}

func callContract(t *testing.T) {
	_, statusCode := httpPost(t, ts.URL+"/accounts/"+invalidAddr, nil)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	malFormedBody := 123
	_, statusCode = httpPost(t, ts.URL+"/accounts", malFormedBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	_, statusCode = httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), malFormedBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	badBody := &accounts.CallData{
		Data: "input",
	}
	_, statusCode = httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid input data")

	a := uint8(1)
	b := uint8(2)
	method := "add"
	abi, _ := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := &accounts.CallData{
		Data: hexutil.Encode(input),
	}
	res, statusCode := httpPost(t, ts.URL+"/accounts/"+contractAddr.String(), reqBody)
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
	// Request body is not a valid JSON
	malformedBody := 123
	_, statusCode := httpPost(t, ts.URL+"/accounts/*", malformedBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "malformed data")

	// Request body is not a valid BatchCallData
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
	_, statusCode = httpPost(t, ts.URL+"/accounts/*", badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid data")

	// Request body has an invalid blockRef
	badBlockRef := &accounts.BatchCallData{
		BlockRef: "0x00",
	}
	_, statusCode = httpPost(t, ts.URL+"/accounts/*", badBlockRef)
	assert.Equal(t, http.StatusInternalServerError, statusCode, "invalid blockRef")

	// Request body has an invalid malformed revision
	_, statusCode = httpPost(t, fmt.Sprintf("%s/accounts/*?revision=%s", ts.URL, "0xZZZ"), badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "revision")

	// Request body has an invalid revision number
	_, statusCode = httpPost(t, ts.URL+"/accounts/*?revision="+invalidNumberRevision, badBody)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid revision")

	// Valid request
	a := uint8(1)
	b := uint8(2)
	method := "add"
	abi, _ := ABI.New([]byte(abiJSON))
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

	res, statusCode := httpPost(t, ts.URL+"/accounts/*", reqBody)
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

	// Valid request
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

	// Request with not enough gas
	tooMuchGasBody := &accounts.BatchCallData{
		Clauses:    accounts.Clauses{},
		Gas:        math.MaxUint64,
		GasPrice:   &big,
		ProvedWork: &big,
		Caller:     &contractAddr,
		GasPayer:   &contractAddr,
		Expiration: 100,
		BlockRef:   "0x00000000aabbccdd",
	}
	_, statusCode = httpPost(t, ts.URL+"/accounts/*", tooMuchGasBody)
	assert.Equal(t, http.StatusForbidden, statusCode)
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
	r, err := io.ReadAll(res.Body)
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
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}

func httpGetAccount(t *testing.T, path string) *accounts.Account {
	res, statusCode := httpGet(t, ts.URL+"/accounts/"+path)
	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode, "get account failed")

	return &acc
}
