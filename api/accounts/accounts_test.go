// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts_test

import (
	"encoding/json"
	"fmt"
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
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
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
	tclient         *thorclient.Client
)

func TestAccount(t *testing.T) {
	initAccountServer(t)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
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
	} {
		t.Run(name, tt)
	}
}

func getAccount(t *testing.T) {
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + invalidAddr)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPGet("/accounts/" + addr.String() + "?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional default `best`
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + addr.String())
	require.NoError(t, err)
	var acc accounts.Account
	if err := json.Unmarshal(res, &acc); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, math.HexOrDecimal256(*value), acc.Balance, "balance should be equal")
	assert.Equal(t, http.StatusOK, statusCode, "OK")
}

func getAccountWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + addr.String() + "?revision=" + revision64Len)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(res), "revision not found")
}

func getAccountWithGenesisRevision(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + addr.String() + "?revision=" + genesisBlock.Header().ID().String())
	require.NoError(t, err)
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
	soloAddress := thor.MustParseAddress("0xf077b491b355E64048cE21E3A6Fc4751eEeA77fa")

	genesisAccount, err := tclient.Account(&soloAddress, thorclient.Revision(genesisBlock.Header().ID().String()))
	require.NoError(t, err)

	finalizedAccount, err := tclient.Account(&soloAddress, thorclient.Revision(tccommon.FinalizedRevision))
	require.NoError(t, err)

	genesisEnergy := (*big.Int)(&genesisAccount.Energy)
	finalizedEnergy := (*big.Int)(&finalizedAccount.Energy)

	assert.Equal(t, genesisEnergy, finalizedEnergy, "finalized energy should equal genesis energy")
}

func getCode(t *testing.T) {
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + invalidAddr + "/code")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/code?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/code")
	require.NoError(t, err)
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

func getCodeWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/code?revision=" + revision64Len)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(res), "revision not found")
}

func getStorage(t *testing.T) {
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + invalidAddr + "/storage/" + storageKey.String())
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad address")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/storage/" + invalidBytes32)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad storage key")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/storage/" + storageKey.String() + "?revision=" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/storage/" + storageKey.String())
	require.NoError(t, err)
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

func getStorageWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/accounts/" + contractAddr.String() + "/storage/" + storageKey.String() + "?revision=" + revision64Len)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(res), "revision not found")
}

func initAccountServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	genesisBlock = thorChain.GenesisBlock()
	claTransfer := tx.NewClause(&addr).WithValue(value)
	claDeploy := tx.NewClause(nil).WithData(bytecode)
	transaction := buildTxWithClauses(thorChain.Repo().ChainTag(), claTransfer, claDeploy)
	contractAddr = thor.CreateContractAddress(transaction.ID(), 1, 0)
	method := "set"
	abi, _ := ABI.New([]byte(abiJSON))
	m, _ := abi.MethodByName(method)
	input, err := m.EncodeInput(storageValue)
	if err != nil {
		t.Fatal(err)
	}
	claCall := tx.NewClause(&contractAddr).WithData(input)
	transactionCall := buildTxWithClauses(thorChain.Repo().ChainTag(), claCall)
	require.NoError(t,
		thorChain.MintTransactions(
			genesis.DevAccounts()[0],
			transaction,
			transactionCall,
		),
	)

	router := mux.NewRouter()
	accounts.New(thorChain.Repo(), thorChain.Stater(), uint64(gasLimit), thor.NoFork, thorChain.Engine()).
		Mount(router, "/accounts")

	ts = httptest.NewServer(router)
}

func buildTxWithClauses(chaiTag byte, clauses ...*tx.Clause) *tx.Transaction {
	builder := new(tx.Builder).
		ChainTag(chaiTag).
		Expiration(10).
		Gas(1000000)
	for _, c := range clauses {
		builder.Clause(c)
	}

	trx := builder.Build()

	return tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)
}

func deployContractWithCall(t *testing.T) {
	badBody := &accounts.CallData{
		Gas:  10000000,
		Data: "abc",
	}
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts", badBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad data")

	reqBody := &accounts.CallData{
		Gas:  10000000,
		Data: hexutil.Encode(bytecode),
	}

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts?revision="+invalidNumberRevision, reqBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")

	//revision is optional defaut `best`
	res, _, err := tclient.RawHTTPClient().RawHTTPPost("/accounts", reqBody)
	require.NoError(t, err)
	var output *accounts.CallResult
	if err := json.Unmarshal(res, &output); err != nil {
		t.Fatal(err)
	}
	assert.False(t, output.Reverted)
}

func callContract(t *testing.T) {
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/"+invalidAddr, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	malFormedBody := 123
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts", malFormedBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/"+contractAddr.String(), malFormedBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid address")

	badBody := &accounts.CallData{
		Data: "input",
	}
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/"+contractAddr.String(), badBody)
	require.NoError(t, err)
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

	// next revisoun should be valid
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/"+contractAddr.String()+"?revision=next", reqBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode, "next revision should be okay")

	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts?revision=next", reqBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode, "next revision should be okay")

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/"+contractAddr.String(), reqBody)
	require.NoError(t, err)
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

func callContractWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/"+contractAddr.String()+"?revision="+revision64Len, nil)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(res), "revision not found")
}

func batchCall(t *testing.T) {
	// Request body is not a valid JSON
	malformedBody := 123
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/*", malformedBody)
	require.NoError(t, err)
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
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*", badBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "invalid data")

	// Request body has an invalid blockRef
	badBlockRef := &accounts.BatchCallData{
		BlockRef: "0x00",
	}
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*", badBlockRef)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, statusCode, "invalid blockRef")

	// Request body has an invalid malformed revision
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost(fmt.Sprintf("/accounts/*?revision=%s", "0xZZZ"), badBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode, "revision")

	// Request body has an invalid revision number
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*?revision="+invalidNumberRevision, badBody)
	require.NoError(t, err)
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

	// 'next' revisoun should be valid
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*?revision=next", reqBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode, "next revision should be okay")

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/*", reqBody)
	require.NoError(t, err)
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
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*", fullBody)
	require.NoError(t, err)
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
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/accounts/*", tooMuchGasBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, statusCode)
}

func batchCallWithNonExistingRevision(t *testing.T) {
	revision64Len := "0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a"

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/accounts/*?revision="+revision64Len, nil)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode, "bad revision")
	assert.Equal(t, "revision: leveldb: not found\n", string(res), "revision not found")
}
