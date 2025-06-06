// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"

	ABI "github.com/vechain/thor/v2/abi"
	tccommon "github.com/vechain/thor/v2/thorclient/common"
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

const (
	storageValue          = byte(1)
	invalidAddr           = "abc"                                                                // invalid address
	invalidBytes32        = "0x000000000000000000000000000000000000000000000000000000000000000g" // invalid bytes32
	invalidNumberRevision = "4294967296"                                                         // invalid block number
)

var (
	gasLimit        = math.MaxUint32
	addr            = thor.BytesToAddress([]byte("to"))
	value           = big.NewInt(10000)
	storageKey      = thor.Bytes32{}
	genesisBlock    *block.Block
	contractAddr    thor.Address
	bytecode        = common.Hex2Bytes("608060405234801561001057600080fd5b50610125806100206000396000f3006080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")
	runtimeBytecode = common.Hex2Bytes("6080604052600436106049576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806324b8ba5f14604e578063bb4e3f4d14607b575b600080fd5b348015605957600080fd5b506079600480360381019080803560ff16906020019092919050505060cf565b005b348015608657600080fd5b5060b3600480360381019080803560ff169060200190929190803560ff16906020019092919050505060ec565b604051808260ff1660ff16815260200191505060405180910390f35b806000806101000a81548160ff021916908360ff16021790555050565b60008183019050929150505600a165627a7a723058201584add23e31d36c569b468097fe01033525686b59bbb263fb3ab82e9553dae50029")
	ts              *httptest.Server
	mimeTypeJSON    = "application/json"
)

func TestAccount(t *testing.T) {
	initAccountServer(t, true)
	defer ts.Close()

	for name, tt := range map[string]func(*testing.T){
		"getAccount":                          getAccount,
		"getAccountWithNonExistingRevision":   getAccountWithNonExistingRevision,
		"getAccountWithGenesisRevision":       getAccountWithGenesisRevision,
		"getAccountWithFinalizedRevision":     getAccountWithFinalizedRevision,
		"getCode":                             getCode,
		"getCodeWithNonExistingRevision":      getCodeWithNonExistingRevision,
		"getStorage":                          getStorage,
		"getStorageWithNonExistingRevision":   getStorageWithNonExistingRevision,
		"deployContractWithCall":              deployContractWithCall,
		"callContract":                        callContract,
		"callContractWithNonExistingRevision": callContractWithNonExistingRevision,
		"batchCall":                           batchCall,
		"batchCallWithNonExistingRevision":    batchCallWithNonExistingRevision,
		"batchCallWithNullClause":             batchCallWithNullClause,
	} {
		t.Run(name, tt)
	}
}

func TestDeprecated(t *testing.T) {
	initAccountServer(t, false)
	defer ts.Close()

	body := &CallData{}
	jsonData, err := json.Marshal(body)
	assert.NoError(t, err)

	res, _ := ts.Client().Post(ts.URL+"/accounts", mimeTypeJSON, bytes.NewReader(jsonData))
	assert.Equal(t, http.StatusGone, res.StatusCode, "invalid address")

	res, _ = ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String(), mimeTypeJSON, bytes.NewReader(jsonData))
	assert.Equal(t, http.StatusGone, res.StatusCode, "invalid address")
}

func getAccount(t *testing.T) {
	resp, err := ts.Client().Get(ts.URL + "/accounts/" + invalidAddr)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad address")

	resp, err = ts.Client().Get(ts.URL + "/accounts/" + addr.String() + "?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad revision")

	//revision is optional default `best`
	res, err := ts.Client().Get(ts.URL + "/accounts/" + addr.String())
	require.NoError(t, err)
	var acc Account
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	if err := json.Unmarshal(body, &acc); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, (*math.HexOrDecimal256)(value), acc.Balance, "balance should be equal")
	assert.Equal(t, http.StatusOK, res.StatusCode, "OK")
}

func getAccountWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, err := ts.Client().Get(ts.URL + "/accounts/" + addr.String() + "?revision=" + revision64Len)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(body), "revision not found")
}

func getAccountWithGenesisRevision(t *testing.T) {
	res, err := ts.Client().Get(ts.URL + "/accounts/" + addr.String() + "?revision=" + genesisBlock.Header().ID().String())
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "bad revision")

	var acc Account
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	if err := json.Unmarshal(body, &acc); err != nil {
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
	soloAddress := thor.MustParseAddress("0xf077b491b355E64048cE21E3A6Fc4751eEeA77fa")

	genesisAccount, err := ts.Client().Get(ts.URL + "/accounts/" + soloAddress.String() + "?revision=" + genesisBlock.Header().ID().String())
	require.NoError(t, err)
	finalizedAccount, err := ts.Client().Get(ts.URL + "/accounts/" + soloAddress.String() + "?revision=" + tccommon.FinalizedRevision)
	require.NoError(t, err)

	var genesisAcc Account
	body, err := io.ReadAll(genesisAccount.Body)
	require.NoError(t, err)
	if err := json.Unmarshal(body, &genesisAcc); err != nil {
		t.Fatal(err)
	}

	var finalizedAcc Account
	body, err = io.ReadAll(finalizedAccount.Body)
	require.NoError(t, err)
	if err := json.Unmarshal(body, &finalizedAcc); err != nil {
		t.Fatal(err)
	}
	genesisEnergy := (*big.Int)(genesisAcc.Energy)
	finalizedEnergy := (*big.Int)(finalizedAcc.Energy)

	assert.Equal(t, genesisEnergy, finalizedEnergy, "finalized energy should equal genesis energy")
}

func getCode(t *testing.T) {
	res, err := ts.Client().Get(ts.URL + "/accounts/" + invalidAddr + "/code")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad address")

	res, err = ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/code?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")

	//revision is optional defaut `best`
	res, err = ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/code")
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	var code map[string]string
	if err := json.Unmarshal(body, &code); err != nil {
		t.Fatal(err)
	}
	c, err := hexutil.Decode(code["code"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, runtimeBytecode, c, "code should be equal")
	assert.Equal(t, http.StatusOK, res.StatusCode, "OK")
}

func getCodeWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, err := ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/code?revision=" + revision64Len)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(body), "revision not found")
}

func getStorage(t *testing.T) {
	res, err := ts.Client().Get(ts.URL + "/accounts/" + invalidAddr + "/storage/" + storageKey.String())
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad address")

	res, err = ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/storage/" + invalidBytes32)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad storage key")

	res, err = ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/storage/" + storageKey.String() + "?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")

	//revision is optional defaut `best`
	res, err = ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/storage/" + storageKey.String())
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var value map[string]string
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	h, err := thor.ParseBytes32(value["value"])
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, thor.BytesToBytes32([]byte{storageValue}), h, "storage should be equal")
	assert.Equal(t, http.StatusOK, res.StatusCode, "OK")
}

func getStorageWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, err := ts.Client().Get(ts.URL + "/accounts/" + contractAddr.String() + "/storage/" + storageKey.String() + "?revision=" + revision64Len)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(body), "revision not found")
}

func initAccountServer(t *testing.T, enabledDeprecated bool) {
	thorChain, err := testchain.NewDefault()
	require.NoError(t, err)

	genesisBlock = thorChain.GenesisBlock()
	claTransfer := tx.NewClause(&addr).WithValue(value)
	claDeploy := tx.NewClause(nil).WithData(bytecode)
	transaction := buildTxWithClauses(tx.TypeLegacy, thorChain.Repo().ChainTag(), claTransfer, claDeploy)
	contractAddr = thor.CreateContractAddress(transaction.ID(), 1, 0)
	method := "set"
	abi, _ := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(storageValue)
	if err != nil {
		t.Fatal(err)
	}
	claCall := tx.NewClause(&contractAddr).WithData(input)
	transactionCall := buildTxWithClauses(tx.TypeLegacy, thorChain.Repo().ChainTag(), claCall)
	require.NoError(t,
		thorChain.MintTransactions(
			genesis.DevAccounts()[0],
			transaction,
			transactionCall,
		),
	)

	router := mux.NewRouter()
	New(thorChain.Repo(), thorChain.Stater(), uint64(gasLimit), &thor.NoFork, thorChain.Engine(), enabledDeprecated).
		Mount(router, "/accounts")

	ts = httptest.NewServer(router)
}

func buildTxWithClauses(txType tx.Type, chainTag byte, clauses ...*tx.Clause) *tx.Transaction {
	trx := tx.NewBuilder(txType).
		ChainTag(chainTag).
		Expiration(10).
		Gas(1000000).
		MaxFeePerGas(big.NewInt(1000)).
		Clauses(clauses).
		Build()
	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}

func deployContractWithCall(t *testing.T) {
	badBody := &CallData{
		Gas:  10000000,
		Data: "abc",
	}
	jsonData, err := json.Marshal(badBody)
	assert.NoError(t, err)

	res, err := ts.Client().Post(ts.URL+"/accounts", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad data")

	reqBody := &CallData{
		Gas:  10000000,
		Data: hexutil.Encode(bytecode),
	}
	jsonData, err = json.Marshal(reqBody)
	assert.NoError(t, err)

	res, err = ts.Client().Post(ts.URL+"/accounts?revision="+invalidNumberRevision, mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")

	//revision is optional defaut `best`
	res, err = ts.Client().Post(ts.URL+"/accounts", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	var output *CallResult
	if err := json.Unmarshal(body, &output); err != nil {
		t.Fatal(err)
	}
	assert.False(t, output.Reverted)
}

func callContract(t *testing.T) {
	res, err := ts.Client().Post(ts.URL+"/accounts/"+invalidAddr, mimeTypeJSON, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid address")

	malFormedBody := 123
	jsonData, err := json.Marshal(malFormedBody)
	assert.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid address")

	res, err = ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String(), mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid address")

	badBody := &CallData{
		Data: "input",
	}
	jsonData, err = json.Marshal(badBody)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String(), mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid input data")

	a := uint8(1)
	b := uint8(2)
	method := "add"
	abi, _ := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(a, b)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := &CallData{
		Data: hexutil.Encode(input),
	}
	jsonData, err = json.Marshal(reqBody)
	require.NoError(t, err)

	// next revision should be valid
	res, err = ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String()+"?revision=next", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "next revision should be okay")

	res, err = ts.Client().Post(ts.URL+"/accounts?revision=next", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "next revision should be okay")

	res, err = ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String(), mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	var output *CallResult
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	if err = json.Unmarshal(body, &output); err != nil {
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
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, a+b, ret)
}

func callContractWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	println("calling", ts.URL+"/accounts/"+contractAddr.String()+"?revision="+revision64Len)
	res, err := ts.Client().Post(ts.URL+"/accounts/"+contractAddr.String()+"?revision="+revision64Len, mimeTypeJSON, bytes.NewReader([]byte("{}")))
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	println(" res is this ", string(body))
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(body), "revision not found")
}

func batchCall(t *testing.T) {
	// Request body is not a valid JSON
	malformedBody := 123
	jsonData, err := json.Marshal(malformedBody)
	require.NoError(t, err)
	res, err := ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "malformed data")

	// Request body is not a valid BatchCallData
	badBody := &BatchCallData{
		Clauses: Clauses{
			&Clause{
				To:    &contractAddr,
				Data:  "data1",
				Value: nil,
			},
			&Clause{
				To:    &contractAddr,
				Data:  "data2",
				Value: nil,
			}},
	}
	jsonData, err = json.Marshal(badBody)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid data")

	// Request body has an invalid blockRef
	badBlockRef := &BatchCallData{
		BlockRef: "0x00",
	}
	jsonData, err = json.Marshal(badBlockRef)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode, "invalid blockRef")

	// Request body has an invalid malformed revision
	jsonData, err = json.Marshal(badBody)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+fmt.Sprintf("/accounts/*?revision=%s", "0xZZZ"), mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "revision")

	// Request body has an invalid revision number
	res, err = ts.Client().Post(ts.URL+"/accounts/*?revision="+invalidNumberRevision, mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "invalid revision")

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
	reqBody := &BatchCallData{
		Clauses: Clauses{
			&Clause{
				To:    &contractAddr,
				Data:  hexutil.Encode(input),
				Value: nil,
			},
			&Clause{
				To:    &contractAddr,
				Data:  hexutil.Encode(input),
				Value: nil,
			}},
	}
	jsonData, err = json.Marshal(reqBody)
	require.NoError(t, err)

	// 'next' revisoun should be valid
	res, err = ts.Client().Post(ts.URL+"/accounts/*?revision=next", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "next revision should be okay")

	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	var results BatchCallResults
	if err = json.Unmarshal(body, &results); err != nil {
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
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Valid request
	big := math.HexOrDecimal256(*big.NewInt(1000))
	fullBody := &BatchCallData{
		Clauses:    Clauses{},
		Gas:        21000,
		GasPrice:   &big,
		ProvedWork: &big,
		Caller:     &contractAddr,
		GasPayer:   &contractAddr,
		Expiration: 100,
		BlockRef:   "0x00000000aabbccdd",
	}
	jsonData, err = json.Marshal(fullBody)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Request with not enough gas
	tooMuchGasBody := &BatchCallData{
		Clauses:    Clauses{},
		Gas:        math.MaxUint64,
		GasPrice:   &big,
		ProvedWork: &big,
		Caller:     &contractAddr,
		GasPayer:   &contractAddr,
		Expiration: 100,
		BlockRef:   "0x00000000aabbccdd",
	}
	jsonData, err = json.Marshal(tooMuchGasBody)
	require.NoError(t, err)
	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader(jsonData))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}

func batchCallWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, err := ts.Client().Post(ts.URL+"/accounts/*?revision="+revision64Len, mimeTypeJSON, bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(body), "revision not found")
}

func batchCallWithNullClause(t *testing.T) {
	res, err := ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader([]byte("{\"clauses\": [null]}")))
	assert.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "null clause")
	assert.Equal(t, "clauses[0]: null not allowed\n", string(body), "null clause")

	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader([]byte("{\"clauses\": [{}, null]}")))
	assert.NoError(t, err)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "null clause")
	assert.Equal(t, "clauses[1]: null not allowed\n", string(body), "null clause")

	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader([]byte("{\"clauses\":null }")))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "null clause")

	res, err = ts.Client().Post(ts.URL+"/accounts/*", mimeTypeJSON, bytes.NewReader([]byte("{\"clauses\":[] }")))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "null clause")
}
