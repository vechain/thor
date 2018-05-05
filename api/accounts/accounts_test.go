package accounts_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	ABI "github.com/vechain/thor/abi"
	"github.com/vechain/thor/api/accounts"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

const (
	testAddress = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
)

type account struct {
	addr    thor.Address
	balance *big.Int
	code    []byte
	storage thor.Bytes32
}

var b, _ = new(big.Int).SetString("10000000000000000000000", 10)
var accs = []struct {
	in, want account
}{
	{
		account{thor.BytesToAddress([]byte("acc1")), b, []byte{0x11, 0x12}, thor.BytesToBytes32([]byte("v1"))},
		account{thor.BytesToAddress([]byte("acc1")), b, []byte{0x11, 0x12}, thor.BytesToBytes32([]byte("v1"))},
	},
	{
		account{thor.BytesToAddress([]byte("acc2")), big.NewInt(100), []byte{0x14, 0x15}, thor.BytesToBytes32([]byte("v2"))},
		account{thor.BytesToAddress([]byte("acc2")), big.NewInt(100), []byte{0x14, 0x15}, thor.BytesToBytes32([]byte("v2"))},
	},
	{
		account{thor.BytesToAddress([]byte("acc3")), big.NewInt(1000), []byte{0x20, 0x21}, thor.BytesToBytes32([]byte("v3"))},
		account{thor.BytesToAddress([]byte("acc3")), big.NewInt(1000), []byte{0x20, 0x21}, thor.BytesToBytes32([]byte("v3"))},
	},
}
var storageKey = thor.BytesToBytes32([]byte("key"))
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

func TestAccount(t *testing.T) {
	ts := initAccountServer(t)
	defer ts.Close()
	getAccount(t, ts)
	callContract(t, ts)
}

func getAccount(t *testing.T, ts *httptest.Server) {
	for _, v := range accs {
		address := v.in.addr
		res := httpGet(t, ts.URL+"/accounts/"+address.String())
		var acc accounts.Account
		if err := json.Unmarshal(res, &acc); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, math.HexOrDecimal256(*v.want.balance), acc.Balance, "balance should be equal")
		res = httpGet(t, ts.URL+"/accounts/"+address.String()+"/code")
		var code map[string]string
		if err := json.Unmarshal(res, &code); err != nil {
			t.Fatal(err)
		}
		c, err := hexutil.Decode(code["code"])
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, v.want.code, c, "code should be equal")

		res = httpGet(t, ts.URL+"/accounts/"+address.String()+"/storage/"+storageKey.String())
		var value map[string]string
		if err := json.Unmarshal(res, &value); err != nil {
			t.Fatal(err)
		}
		h, err := thor.ParseBytes32(value["value"])
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, v.want.storage, h, "storage should be equal")

	}
}

func initAccountServer(t *testing.T) *httptest.Server {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	st, _ := stateC.NewState(thor.Bytes32{})
	for _, v := range accs {
		address := v.in.addr
		st.SetBalance(address, v.in.balance)
		st.SetCode(address, v.in.code)
		st.SetStorage(address, storageKey, v.in.storage)
	}
	st.SetCode(contractAddr, code)
	stateRoot, _ := st.Stage().Commit()
	gene, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := gene.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	best := chain.BestBlock()
	bl := new(block.Builder).
		ParentID(best.Header().ID()).
		StateRoot(stateRoot).
		Build()
	if _, err := chain.AddBlock(bl, nil, true); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	accounts.New(chain, stateC).Mount(router, "/accounts/")
	ts := httptest.NewServer(router)
	return ts
}

func callContract(t *testing.T, ts *httptest.Server) {
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
