// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tracers/logger"
	"github.com/vechain/thor/v2/tx"

	// Force-load the tracer native engines to trigger registration
	_ "github.com/vechain/thor/v2/tracers/js"
	_ "github.com/vechain/thor/v2/tracers/native"
)

var (
	ts          *httptest.Server
	blk         *block.Block
	transaction *tx.Transaction
	debug       *Debug
	tclient     *thorclient.Client
)

func TestDebug(t *testing.T) {
	initDebugServer(t)
	defer ts.Close()

	// /tracers endpoint
	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"testTraceClauseWithInvalidTracerName":     testTraceClauseWithInvalidTracerName,
		"testTraceClauseWithEmptyTracerTarget":     testTraceClauseWithEmptyTracerTarget,
		"testTraceClauseWithBadBlockID":            testTraceClauseWithBadBlockID,
		"testTraceClauseWithNonExistingBlockID":    testTraceClauseWithNonExistingBlockID,
		"testTraceClauseWithBadTxID":               testTraceClauseWithBadTxID,
		"testTraceClauseWithNonExistingTx":         testTraceClauseWithNonExistingTx,
		"testTraceClauseWithBadClauseIndex":        testTraceClauseWithBadClauseIndex,
		"testTraceClauseWithTxIndexOutOfBound":     testTraceClauseWithTxIndexOutOfBound,
		"testTraceClauseWithClauseIndexOutOfBound": testTraceClauseWithClauseIndexOutOfBound,
		"testTraceClauseWithCustomTracer":          testTraceClauseWithCustomTracer,
		"testTraceClause":                          testTraceClause,
	} {
		t.Run(name, tt)
	}

	// /tracers/call endpoint
	for name, tt := range map[string]func(*testing.T){
		"testHandleTraceCallWithMalformedBodyRequest":        testHandleTraceCallWithMalformedBodyRequest,
		"testHandleTraceCallWithEmptyTraceCallOption":        testHandleTraceCallWithEmptyTraceCallOption,
		"testHandleTraceCall":                                testHandleTraceCall,
		"testHandleTraceCallWithValidRevisions":              testHandleTraceCallWithValidRevisions,
		"testHandleTraceCallWithRevisionAsNonExistingHeight": testHandleTraceCallWithRevisionAsNonExistingHeight,
		"testHandleTraceCallWithRevisionAsNonExistingID":     testHandleTraceCallWithRevisionAsNonExistingID,
		"testHandleTraceCallWithMalfomredRevision":           testHandleTraceCallWithMalfomredRevision,
		"testHandleTraceCallWithInsufficientGas":             testHandleTraceCallWithInsufficientGas,
		"testHandleTraceCallWithBadBlockRef":                 testHandleTraceCallWithBadBlockRef,
		"testHandleTraceCallWithInvalidLengthBlockRef":       testHandleTraceCallWithInvalidLengthBlockRef,
		"testTraceCallNextBlock":                             testTraceCallNextBlock,
	} {
		t.Run(name, tt)
	}

	// /storage/range endpoint
	for name, tt := range map[string]func(*testing.T){
		"testStorageRangeWithError":     testStorageRangeWithError,
		"testStorageRange":              testStorageRange,
		"testStorageRangeDefaultOption": testStorageRangeDefaultOption,
	} {
		t.Run(name, tt)
	}
}

func TestStorageRangeFunc(t *testing.T) {
	db := muxdb.NewMem()
	state := state.New(db, thor.Bytes32{}, 0, 0, 0)

	// Create an account and set storage values
	addr := thor.BytesToAddress([]byte("account1"))
	key1 := thor.BytesToBytes32([]byte("key1"))
	value1 := thor.BytesToBytes32([]byte("value1"))
	key2 := thor.BytesToBytes32([]byte("key2"))
	value2 := thor.BytesToBytes32([]byte("value2"))

	state.SetRawStorage(addr, key1, value1[:])
	state.SetRawStorage(addr, key2, value2[:])

	trie, err := state.BuildStorageTrie(addr)
	if err != nil {
		t.Fatal(err)
	}
	start, err := hexutil.Decode("0x00")
	if err != nil {
		t.Fatal(err)
	}

	storageRangeRes, err := storageRangeAt(trie, start, 1)
	assert.NoError(t, err)
	assert.NotNil(t, storageRangeRes.NextKey)
	storage := storageRangeRes.Storage
	assert.Equal(t, 1, len(storage))
}

func TestStorageRangeMaxResult(t *testing.T) {
	db := muxdb.NewMem()
	state := state.New(db, thor.Bytes32{}, 0, 0, 0)

	addr := thor.BytesToAddress([]byte("account1"))
	for i := 0; i < 1001; i++ {
		key := thor.BytesToBytes32([]byte(fmt.Sprintf("key%d", i)))
		value := thor.BytesToBytes32([]byte(fmt.Sprintf("value%d", i)))
		state.SetRawStorage(addr, key, value[:])
	}

	trie, err := state.BuildStorageTrie(addr)
	if err != nil {
		t.Fatal(err)
	}
	start, err := hexutil.Decode("0x00")
	if err != nil {
		t.Fatal(err)
	}

	storageRangeRes, err := storageRangeAt(trie, start, 1001)
	assert.NoError(t, err)
	assert.Equal(t, 1001, len(storageRangeRes.Storage))

	storageRangeRes, err = storageRangeAt(trie, start, 1000)
	assert.NoError(t, err)
	assert.Equal(t, 1000, len(storageRangeRes.Storage))

	storageRangeRes, err = storageRangeAt(trie, start, 10)
	assert.NoError(t, err)
	assert.Equal(t, 10, len(storageRangeRes.Storage))
}

func testTraceClauseWithInvalidTracerName(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", &TraceClauseOption{Name: "non-existent"}, 403)
	assert.Contains(t, res, "unable to create custom tracer")
}

func testTraceClauseWithEmptyTracerTarget(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", &TraceClauseOption{Name: "structLogger"}, 400)
	assert.Equal(t, "target: unsupported", strings.TrimSpace(res))
}

func testTraceClauseWithBadBlockID(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: "badBlockId/x/x",
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, "target[0]: invalid length", strings.TrimSpace(res))
}

func testTraceClauseWithNonExistingBlockID(t *testing.T) {
	_, _, _, err := debug.prepareClauseEnv(context.Background(), datagen.RandomHash(), 1, 1)

	assert.Error(t, err)
}

func testTraceClauseWithBadTxID(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/badTxId/x", blk.Header().ID()),
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `target[1]: strconv.ParseUint: parsing "badTxId": invalid syntax`, strings.TrimSpace(res))
}

func testTraceClauseWithNonExistingTx(t *testing.T) {
	nonExistingTxID := "0x4500ade0d72115abfc77571aef752df45ba5e87ca81fbd67fbfc46d455b17f91"
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/%s/x", blk.Header().ID(), nonExistingTxID),
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 403)
	assert.Equal(t, "transaction not found", strings.TrimSpace(res))
}

func testTraceClauseWithBadClauseIndex(t *testing.T) {
	// Clause index is not a number
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/%s/x", blk.Header().ID(), transaction.ID()),
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `target[2]: strconv.ParseUint: parsing "x": invalid syntax`, strings.TrimSpace(res))

	// Clause index is out of range
	traceClauseOption = &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/%s/%d", blk.Header().ID(), transaction.ID(), uint64(math.MaxUint64)),
	}
	res = httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `invalid target[2]`, strings.TrimSpace(res))
}

func testTraceClauseWithCustomTracer(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Target: fmt.Sprintf("%s/%s/1", blk.Header().ID(), transaction.ID()),
		Name:   "nonExistingTracer",
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 403)
	assert.Contains(t, strings.TrimSpace(res), "create custom tracer: ReferenceError: nonExistingTracer is not defined")

	traceClauseOption = &TraceClauseOption{
		Target: fmt.Sprintf("%s/%s/1", blk.Header().ID(), transaction.ID()),
		Name:   "4byteTracer",
	}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  nil,
	}
	res = httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceClause(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/%s/1", blk.Header().ID(), transaction.ID()),
	}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  make([]logger.StructLogRes, 0),
	}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceClauseWithTxIndexOutOfBound(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/10/1", blk.Header().ID()),
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 403)

	assert.Equal(t, "tx index out of range", strings.TrimSpace(res))
}

func testTraceClauseWithClauseIndexOutOfBound(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "structLogger",
		Target: fmt.Sprintf("%s/%s/10", blk.Header().ID(), transaction.ID()),
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers", traceClauseOption, 403)

	assert.Equal(t, "clause index out of range", strings.TrimSpace(res))
}

func testHandleTraceCallWithMalformedBodyRequest(t *testing.T) {
	badBodyRequest := "badBodyRequest"
	httpPostAndCheckResponseStatus(t, "/debug/tracers/call", badBodyRequest, 400)
}

func testHandleTraceCallWithEmptyTraceCallOption(t *testing.T) {
	traceCallOption := &TraceCallOption{Name: "structLogger"}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  make([]logger.StructLogRes, 0),
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call", traceCallOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceCallNextBlock(t *testing.T) {
	traceCallOption := &TraceCallOption{Name: "structLogger"}
	httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision=next", traceCallOption, 200)
}

func testHandleTraceCall(t *testing.T) {
	addr := datagen.RandAddress()
	provedWork := math.HexOrDecimal256(*big.NewInt(1000))
	traceCallOption := &TraceCallOption{
		Name:       "structLogger",
		To:         &addr,
		Value:      &math.HexOrDecimal256{},
		Data:       "0x00",
		Gas:        21000,
		GasPrice:   &math.HexOrDecimal256{},
		ProvedWork: &provedWork,
		Caller:     &addr,
		GasPayer:   &addr,
		Expiration: 10,
		BlockRef:   "0x0000000000000000",
	}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  make([]logger.StructLogRes, 0),
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call", traceCallOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testHandleTraceCallWithValidRevisions(t *testing.T) {
	revisions := []string{
		blk.Header().ID().String(),
		"1",
		"finalized",
		"best",
	}

	for _, revision := range revisions {
		expectedExecutionResult := &logger.ExecutionResult{
			Gas:         0,
			Failed:      false,
			ReturnValue: "",
			StructLogs:  make([]logger.StructLogRes, 0),
		}

		res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision="+revision, &TraceCallOption{Name: "structLogger"}, 200)

		var parsedExecutionRes *logger.ExecutionResult
		if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, expectedExecutionResult, parsedExecutionRes, "revision: %s", revision)
	}
}

func testHandleTraceCallWithRevisionAsNonExistingHeight(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision=12345", &TraceCallOption{}, 400)

	assert.Equal(t, "revision: not found", strings.TrimSpace(res))
}

func testHandleTraceCallWithRevisionAsNonExistingID(t *testing.T) {
	nonExistingRevision := "0x4500ade0d72115abfc77571aef752df45ba5e87ca81fbd67fbfc46d455b17f91"

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision="+nonExistingRevision, &TraceCallOption{}, 400)

	assert.Equal(t, "revision: leveldb: not found", strings.TrimSpace(res))
}

func testHandleTraceCallWithMalfomredRevision(t *testing.T) {
	// Revision is a malformed byte array
	traceCallOption := &TraceCallOption{}
	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision=012345678901234567890123456789012345678901234567890123456789012345", traceCallOption, 400)
	assert.Equal(t, "revision: invalid prefix", strings.TrimSpace(res))

	// Revision is a not accepted string
	res = httpPostAndCheckResponseStatus(t, "/debug/tracers/call?revision=badRevision", traceCallOption, 400)
	assert.Equal(t, `revision: strconv.ParseUint: parsing "badRevision": invalid syntax`, strings.TrimSpace(res))

	// Revision number is out of range
	res = httpPostAndCheckResponseStatus(t, fmt.Sprintf("/debug/tracers/call?revision=%d", uint64(math.MaxUint64)), traceCallOption, 400)
	assert.Equal(t, "revision: block number out of max uint32", strings.TrimSpace(res))
}

func testHandleTraceCallWithInsufficientGas(t *testing.T) {
	addr := datagen.RandAddress()
	traceCallOption := &TraceCallOption{
		Name:       "structLogger",
		To:         &addr,
		Value:      &math.HexOrDecimal256{},
		Data:       "0x00",
		Gas:        70000,
		GasPrice:   &math.HexOrDecimal256{},
		Caller:     &addr,
		GasPayer:   &addr,
		Expiration: 10,
		BlockRef:   "0x0000000000000000",
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call", traceCallOption, 403)

	assert.Equal(t, "gas: exceeds limit", strings.TrimSpace(res))
}

func testHandleTraceCallWithBadBlockRef(t *testing.T) {
	addr := datagen.RandAddress()
	traceCallOption := &TraceCallOption{
		Name:       "structLogger",
		To:         &addr,
		Value:      &math.HexOrDecimal256{},
		Data:       "0x00",
		Gas:        10,
		GasPrice:   &math.HexOrDecimal256{},
		Caller:     &addr,
		GasPayer:   &addr,
		Expiration: 10,
		BlockRef:   "jh000000000000000",
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call", traceCallOption, 500)

	assert.Equal(t, "blockRef: hex string without 0x prefix", strings.TrimSpace(res))
}

func testHandleTraceCallWithInvalidLengthBlockRef(t *testing.T) {
	addr := datagen.RandAddress()
	traceCallOption := &TraceCallOption{
		Name:       "structLogger",
		To:         &addr,
		Value:      &math.HexOrDecimal256{},
		Data:       "0x00",
		Gas:        10,
		GasPrice:   &math.HexOrDecimal256{},
		Caller:     &addr,
		GasPayer:   &addr,
		Expiration: 10,
		BlockRef:   "0x00",
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/tracers/call", traceCallOption, 500)

	assert.Equal(t, "blockRef: invalid length", strings.TrimSpace(res))
}

func testStorageRangeWithError(t *testing.T) {
	// Error case 1: empty StorageRangeOption
	opt := &StorageRangeOption{}
	httpPostAndCheckResponseStatus(t, "/debug/storage-range", opt, 400)

	// Error case 2: bad StorageRangeOption
	badBodyRequest := 123
	httpPostAndCheckResponseStatus(t, "/debug/storage-range", badBodyRequest, 400)

	badMaxResult := &StorageRangeOption{MaxResult: 1001}
	httpPostAndCheckResponseStatus(t, "/debug/storage-range", badMaxResult, 400)
}

func testStorageRange(t *testing.T) {
	opt := StorageRangeOption{
		Address:   datagen.RandAddress(),
		KeyStart:  "0x00",
		MaxResult: 100,
		Target:    fmt.Sprintf("%s/%s/0", blk.Header().ID(), transaction.ID()),
	}
	expectedStorageRangeResult := &StorageRangeResult{
		Storage: make(StorageMap, 0),
		NextKey: nil,
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/storage-range", &opt, 200)

	var parsedExecutionRes *StorageRangeResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedStorageRangeResult, parsedExecutionRes)
}

func testStorageRangeDefaultOption(t *testing.T) {
	opt := StorageRangeOption{
		Address: builtin.Energy.Address,
		Target:  fmt.Sprintf("%s/%s/0", blk.Header().ID(), transaction.ID()),
	}

	res := httpPostAndCheckResponseStatus(t, "/debug/storage-range", &opt, 200)

	var storageRangeRes *StorageRangeResult
	if err := json.Unmarshal([]byte(res), &storageRangeRes); err != nil {
		t.Fatal(err)
	}
	assert.NotZero(t, len(storageRangeRes.Storage))
}

func initDebugServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))

	// Adding an empty clause transaction to the block to cover the case of
	// scanning multiple txs before getting the right one
	noClausesTx := new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(21000).
		Build()
	noClausesTx = tx.MustSign(noClausesTx, genesis.DevAccounts()[0].PrivateKey)

	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	transaction = new(tx.Builder).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	transaction = tx.MustSign(transaction, genesis.DevAccounts()[0].PrivateKey)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], transaction, noClausesTx))
	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0]))

	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	blk = allBlocks[1]

	forkConfig := thor.GetForkConfig(blk.Header().ID())
	router := mux.NewRouter()
	debug = New(thorChain.Repo(), thorChain.Stater(), forkConfig, 21000, true, thorChain.Engine(), []string{"all"}, false)
	debug.Mount(router, "/debug")
	ts = httptest.NewServer(router)
}

func httpPostAndCheckResponseStatus(t *testing.T, url string, obj interface{}, responseStatusCode int) string {
	body, status, err := tclient.RawHTTPClient().RawHTTPPost(url, obj)
	require.NoError(t, err)
	require.Equal(t, responseStatusCode, status)

	return string(body)
}
func TestCreateTracer(t *testing.T) {
	debug := &Debug{}

	// all
	debug.allowedTracers = map[string]struct{}{"all": {}}
	tr, err := debug.createTracer("", nil)
	assert.Nil(t, err)
	assert.IsType(t, &logger.StructLogger{}, tr)
	_, err = debug.createTracer("{result:()=>{}, fault:()=>{}}", nil)
	assert.EqualError(t, err, "tracer is not defined")

	tr, err = debug.createTracer("structLogger", nil)
	assert.Nil(t, err)
	assert.IsType(t, &logger.StructLogger{}, tr)

	// none
	debug.allowedTracers = map[string]struct{}{}
	_, err = debug.createTracer("structLogger", nil)
	assert.EqualError(t, err, "creating tracer is not allowed: structLogger")

	// custom tracer
	debug.allowCustomTracer = true
	_, err = debug.createTracer("{result:()=>{}, fault:()=>{}}", nil)
	assert.Nil(t, err)
}
