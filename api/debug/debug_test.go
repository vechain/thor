// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tracers/logger"
	"github.com/vechain/thor/v2/tx"

	// Force-load the tracer native engines to trigger registration
	_ "github.com/vechain/thor/v2/tracers/js"
	_ "github.com/vechain/thor/v2/tracers/native"
)

var ts *httptest.Server
var blk *block.Block
var transaction *tx.Transaction
var debug *Debug

func TestDebug(t *testing.T) {
	initDebugServer(t)
	defer ts.Close()

	// /tracers endpoint
	for name, tt := range map[string]func(*testing.T){
		"testTraceClauseWithEmptyTracerName":       testTraceClauseWithEmptyTracerName,
		"testTraceClauseWithInvalidTracerName":     testTraceClauseWithInvalidTracerName,
		"testTraceClauseWithEmptyTracerTarget":     testTraceClauseWithEmptyTracerTarget,
		"testTraceClauseWithBadBlockId":            testTraceClauseWithBadBlockId,
		"testTraceClauseWithNonExistingBlockId":    testTraceClauseWithNonExistingBlockId,
		"testTraceClauseWithBadTxId":               testTraceClauseWithBadTxId,
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
		"testHandleTraceCallWithRevisionAsNonExistingId":     testHandleTraceCallWithRevisionAsNonExistingId,
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

func testTraceClauseWithEmptyTracerName(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", &TraceClauseOption{}, 403)
	assert.Equal(t, "tracer name must be defined", strings.TrimSpace(res))

	res = httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", &TraceClauseOption{Name: " "}, 403)
	assert.Equal(t, "tracer name must be defined", strings.TrimSpace(res))
}

func testTraceClauseWithInvalidTracerName(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", &TraceClauseOption{Name: "non-existent"}, 403)
	assert.Contains(t, res, "unable to create custom tracer")
}

func testTraceClauseWithEmptyTracerTarget(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", &TraceClauseOption{Name: "logger"}, 400)
	assert.Equal(t, "target: unsupported", strings.TrimSpace(res))
}

func testTraceClauseWithBadBlockId(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: "badBlockId/x/x",
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, "target[0]: invalid length", strings.TrimSpace(res))
}

func testTraceClauseWithNonExistingBlockId(t *testing.T) {
	_, _, _, err := debug.prepareClauseEnv(context.Background(), randBytes32(), 1, 1)

	assert.Error(t, err)
}

func testTraceClauseWithBadTxId(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/badTxId/x", blk.Header().ID()),
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `target[1]: strconv.ParseUint: parsing "badTxId": invalid syntax`, strings.TrimSpace(res))
}

func testTraceClauseWithNonExistingTx(t *testing.T) {
	nonExistingTxId := "0x4500ade0d72115abfc77571aef752df45ba5e87ca81fbd67fbfc46d455b17f91"
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/%s/x", blk.Header().ID(), nonExistingTxId),
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 403)
	assert.Equal(t, "transaction not found", strings.TrimSpace(res))
}

func testTraceClauseWithBadClauseIndex(t *testing.T) {
	// Clause index is not a number
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/%s/x", blk.Header().ID(), transaction.ID()),
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `target[2]: strconv.ParseUint: parsing "x": invalid syntax`, strings.TrimSpace(res))

	// Clause index is out of range
	traceClauseOption = &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/%s/%d", blk.Header().ID(), transaction.ID(), uint64(math.MaxUint64)),
	}
	res = httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 400)
	assert.Equal(t, `invalid target[2]`, strings.TrimSpace(res))
}

func testTraceClauseWithCustomTracer(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Target: fmt.Sprintf("%s/%s/1", blk.Header().ID(), transaction.ID()),
		Name:   "nonExistingTracer",
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 403)
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
	res = httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceClause(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/%s/1", blk.Header().ID(), transaction.ID()),
	}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  make([]logger.StructLogRes, 0),
	}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceClauseWithTxIndexOutOfBound(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/10/1", blk.Header().ID()),
	}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 403)

	assert.Equal(t, "tx index out of range", strings.TrimSpace(res))
}

func testTraceClauseWithClauseIndexOutOfBound(t *testing.T) {
	traceClauseOption := &TraceClauseOption{
		Name:   "logger",
		Target: fmt.Sprintf("%s/%s/10", blk.Header().ID(), transaction.ID()),
	}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers", traceClauseOption, 403)

	assert.Equal(t, "clause index out of range", strings.TrimSpace(res))
}

func testHandleTraceCallWithMalformedBodyRequest(t *testing.T) {
	badBodyRequest := "badBodyRequest"
	httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", badBodyRequest, 400)
}

func testHandleTraceCallWithEmptyTraceCallOption(t *testing.T) {
	traceCallOption := &TraceCallOption{Name: "logger"}
	expectedExecutionResult := &logger.ExecutionResult{
		Gas:         0,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  make([]logger.StructLogRes, 0),
	}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", traceCallOption, 200)

	var parsedExecutionRes *logger.ExecutionResult
	if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expectedExecutionResult, parsedExecutionRes)
}

func testTraceCallNextBlock(t *testing.T) {
	traceCallOption := &TraceCallOption{Name: "logger"}
	httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision=next", traceCallOption, 200)
}

func testHandleTraceCall(t *testing.T) {
	addr := randAddress()
	provedWork := math.HexOrDecimal256(*big.NewInt(1000))
	traceCallOption := &TraceCallOption{
		Name:       "logger",
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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", traceCallOption, 200)

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

		res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision="+revision, &TraceCallOption{Name: "logger"}, 200)

		var parsedExecutionRes *logger.ExecutionResult
		if err := json.Unmarshal([]byte(res), &parsedExecutionRes); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, expectedExecutionResult, parsedExecutionRes, "revision: %s", revision)
	}
}

func testHandleTraceCallWithRevisionAsNonExistingHeight(t *testing.T) {
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision=12345", &TraceCallOption{}, 400)

	assert.Equal(t, "revision: not found", strings.TrimSpace(res))
}

func testHandleTraceCallWithRevisionAsNonExistingId(t *testing.T) {
	nonExistingRevision := "0x4500ade0d72115abfc77571aef752df45ba5e87ca81fbd67fbfc46d455b17f91"

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision="+nonExistingRevision, &TraceCallOption{}, 400)

	assert.Equal(t, "revision: leveldb: not found", strings.TrimSpace(res))
}

func testHandleTraceCallWithMalfomredRevision(t *testing.T) {
	// Revision is a malformed byte array
	traceCallOption := &TraceCallOption{}
	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision=012345678901234567890123456789012345678901234567890123456789012345", traceCallOption, 400)
	assert.Equal(t, "revision: invalid prefix", strings.TrimSpace(res))

	// Revision is a not accepted string
	res = httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call?revision=badRevision", traceCallOption, 400)
	assert.Equal(t, `revision: strconv.ParseUint: parsing "badRevision": invalid syntax`, strings.TrimSpace(res))

	// Revision number is out of range
	res = httpPostAndCheckResponseStatus(t, fmt.Sprintf("%s/debug/tracers/call?revision=%d", ts.URL, uint64(math.MaxUint64)), traceCallOption, 400)
	assert.Equal(t, "revision: block number out of max uint32", strings.TrimSpace(res))
}

func testHandleTraceCallWithInsufficientGas(t *testing.T) {
	addr := randAddress()
	traceCallOption := &TraceCallOption{
		Name:       "logger",
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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", traceCallOption, 403)

	assert.Equal(t, "gas: exceeds limit", strings.TrimSpace(res))
}

func testHandleTraceCallWithBadBlockRef(t *testing.T) {
	addr := randAddress()
	traceCallOption := &TraceCallOption{
		Name:       "logger",
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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", traceCallOption, 500)

	assert.Equal(t, "blockRef: hex string without 0x prefix", strings.TrimSpace(res))
}

func testHandleTraceCallWithInvalidLengthBlockRef(t *testing.T) {
	addr := randAddress()
	traceCallOption := &TraceCallOption{
		Name:       "logger",
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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/tracers/call", traceCallOption, 500)

	assert.Equal(t, "blockRef: invalid length", strings.TrimSpace(res))
}

func testStorageRangeWithError(t *testing.T) {
	// Error case 1: empty StorageRangeOption
	opt := &StorageRangeOption{}
	httpPostAndCheckResponseStatus(t, ts.URL+"/debug/storage-range", opt, 400)

	// Error case 2: bad StorageRangeOption
	badBodyRequest := 123
	httpPostAndCheckResponseStatus(t, ts.URL+"/debug/storage-range", badBodyRequest, 400)

	badMaxResult := &StorageRangeOption{MaxResult: 1001}
	httpPostAndCheckResponseStatus(t, ts.URL+"/debug/storage-range", badMaxResult, 400)
}

func testStorageRange(t *testing.T) {
	opt := StorageRangeOption{
		Address:   randAddress(),
		KeyStart:  "0x00",
		MaxResult: 100,
		Target:    fmt.Sprintf("%s/%s/0", blk.Header().ID(), transaction.ID()),
	}
	expectedStorageRangeResult := &StorageRangeResult{
		Storage: make(StorageMap, 0),
		NextKey: nil,
	}

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/storage-range", &opt, 200)

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

	res := httpPostAndCheckResponseStatus(t, ts.URL+"/debug/storage-range", &opt, 200)

	var storageRangeRes *StorageRangeResult
	if err := json.Unmarshal([]byte(res), &storageRangeRes); err != nil {
		t.Fatal(err)
	}
	assert.NotZero(t, len(storageRangeRes.Storage))
}

func initDebugServer(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	addr := thor.BytesToAddress([]byte("to"))

	// Adding an empty clause transaction to the block to cover the case of
	// scanning multiple txs before getting the right one
	noClausesTx := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		Expiration(10).
		Gas(21000).
		Build()
	sig, err := crypto.Sign(noClausesTx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	noClausesTx = noClausesTx.WithSignature(sig)

	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	transaction = new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err = crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	transaction = transaction.WithSignature(sig)
	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(noClausesTx)
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(transaction)
	if err != nil {
		t.Fatal(err)
	}
	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	blk = b
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

	forkConfig := thor.GetForkConfig(b.Header().ID())
	router := mux.NewRouter()
	allTracersEnabled := map[string]interface{}{"all": new(interface{})}
	debug = New(repo, stater, forkConfig, 21000, true, solo.NewBFTEngine(repo), allTracersEnabled, false)
	debug.Mount(router, "/debug")
	ts = httptest.NewServer(router)
}

func httpPostAndCheckResponseStatus(t *testing.T, url string, obj interface{}, responseStatusCode int) string {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data)) // nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, responseStatusCode, res.StatusCode)
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(r)
}

func randAddress() (addr thor.Address) {
	rand.Read(addr[:])
	return
}

func randBytes32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}
