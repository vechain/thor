# JSON-RPC Server Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a self-built, HTTP-only, reflection-based JSON-RPC 2.0 server to thor as a new `api/jsonrpc/` subpackage, exposing `eth_chainId` / `eth_blockNumber` / `eth_getBalance` (+ `net_version` / `web3_clientVersion`) as a working bootstrap.

**Architecture:** A minimal `Server` holds a reflection `serviceRegistry` (namespace → method → callback), mirroring go-ethereum `rpc/service.go`. Business methods are plain exported Go methods on `*ethAPI` / `*netAPI` / `*web3API`; `RegisterName("eth", svc)` reflects them into `eth_<method>` endpoints. One HTTP handler (`POST /rpc`) decodes the JSON-RPC envelope (single or batch), dispatches through the registry, and writes the response. It mounts on thor's existing `mux.Router` via `.Mount(router, "/rpc")`, inheriting all current middleware.

**Tech Stack:** Go, `reflect`, `encoding/json`, `gorilla/mux`, `github.com/ethereum/go-ethereum/common` + `common/hexutil`, thor `api/restutil`, `chain`, `state`, `bft`, testify, `test/testchain`.

**Reference spec:** `docs/jsonrpc/jsonrpc-server-design.md` (§5 is the source of the code below; §2.5 documents middleware side effects — no code impact for bootstrap).

## Global Constraints

- Module path is `github.com/vechain/thor/v2` — all internal imports use this prefix.
- Every new `.go` file starts with the LGPL header:
  ```go
  // Copyright (c) 2026 The VeChainThor developers

  // Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
  // file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
  ```
- go-ethereum is pinned to a 2018 `v1.8.x` fork; use ONLY `github.com/ethereum/go-ethereum/common` and `.../common/hexutil`. Do NOT import `github.com/ethereum/go-ethereum/rpc` (unused across the repo; keep it that way).
- HTTP-only. No WebSocket, no subscriptions, no filters, no write path in this plan.
- Do not modify any existing REST subpackage or its behavior. JSON-RPC shares the same `chain.Repository` / `state.Stater` — never build a separate cache/snapshot.
- Big integers cross the wire as `*hexutil.Big`; counters as `hexutil.Uint64`. Never return a bare `*big.Int`.
- `restutil` verified signatures: `WriteJSON(w http.ResponseWriter, obj any) error`; `HandlerFunc = func(http.ResponseWriter, *http.Request) error`; `WrapHandlerFunc(HandlerFunc) http.HandlerFunc`; `ParseRevision(string, allowNext bool) (*Revision, error)`; `GetSummaryAndState(rev *Revision, repo *chain.Repository, bft bft.Committer, stater *state.Stater, forkConfig *thor.ForkConfig) (*chain.BlockSummary, *state.State, error)`.
- Test command prefix: `go test ./api/jsonrpc/ -run <Name> -v`.

## File Structure

- Create `api/jsonrpc/json.go` — JSON-RPC 2.0 envelope, error codes, `jsonError`, `DataError`, `errorResponse`, `toJSONError`.
- Create `api/jsonrpc/service.go` — reflection core: `callback`, `serviceRegistry`, `registerName`, `callback` lookup, `suitableCallbacks`, `newCallback`, `parseArgs`, `call`, `isErrorType`, `formatName`.
- Create `api/jsonrpc/server.go` — `Server`, `NewServer`, `RegisterName`, `handleMsg`.
- Create `api/jsonrpc/backend.go` — `backend` struct + `stateForRevision`.
- Create `api/jsonrpc/eth.go` — `ethAPI` with `ChainId` / `BlockNumber` / `GetBalance`.
- Create `api/jsonrpc/net.go` — `netAPI` with `Version`.
- Create `api/jsonrpc/web3.go` — `web3API` with `ClientVersion`.
- Create `api/jsonrpc/jsonrpc.go` — `JSONRPC`, `New`, `Mount`, `handleHTTP`.
- Create test files: `json_test.go`, `service_test.go`, `server_test.go`, `methods_test.go`, `http_test.go`.
- Modify `cmd/thor/httpserver/api_server.go` — add `EnableRPC bool` to `APIConfig`; guarded `.Mount(router, "/rpc")`.
- Modify `cmd/thor/flags.go` — add `enableRPCFlag`.
- Modify `cmd/thor/utils.go` — set `EnableRPC` in `makeAPIConfig`.
- Modify `cmd/thor/main.go` — register `enableRPCFlag` in the two `Flags` slices.

---

### Task 1: JSON-RPC envelope and error model

**Files:**
- Create: `api/jsonrpc/json.go`
- Test: `api/jsonrpc/json_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - const `jsonrpcVersion = "2.0"`; error codes `errcodeParse=-32700`, `errcodeInvalidRequest=-32600`, `errcodeMethodNotFound=-32601`, `errcodeInvalidParams=-32602`, `errcodeInternal=-32603`, `errcodeDefault=-32000`.
  - `type jsonrpcMessage struct{ Version string; ID json.RawMessage; Method string; Params json.RawMessage; Result interface{}; Error *jsonError }`
  - `type jsonError struct{ Code int; Message string; Data interface{} }` with `func (e *jsonError) Error() string`.
  - `type DataError interface{ error; ErrorCode() int; ErrorData() interface{} }`
  - `func errorResponse(id json.RawMessage, je *jsonError) *jsonrpcMessage`
  - `func toJSONError(err error) *jsonError`

- [ ] **Step 1: Write the failing test**

Create `api/jsonrpc/json_test.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type dataErr struct{}

func (dataErr) Error() string           { return "boom" }
func (dataErr) ErrorCode() int          { return -32050 }
func (dataErr) ErrorData() interface{}  { return map[string]int{"x": 1} }

func TestToJSONError(t *testing.T) {
	// plain jsonError passes through unchanged
	je := &jsonError{Code: errcodeInvalidParams, Message: "bad"}
	assert.Same(t, je, toJSONError(je))

	// generic error maps to -32000
	got := toJSONError(errors.New("plain"))
	assert.Equal(t, errcodeDefault, got.Code)
	assert.Equal(t, "plain", got.Message)
	assert.Nil(t, got.Data)

	// DataError carries code + data
	got = toJSONError(dataErr{})
	assert.Equal(t, -32050, got.Code)
	assert.Equal(t, map[string]int{"x": 1}, got.Data)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/jsonrpc/ -run TestToJSONError -v`
Expected: FAIL — build error, `undefined: jsonError` / package has no such symbols.

- [ ] **Step 3: Write minimal implementation**

Create `api/jsonrpc/json.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "encoding/json"

const jsonrpcVersion = "2.0"

// JSON-RPC 2.0 error codes (see go-ethereum rpc/errors.go).
const (
	errcodeParse          = -32700
	errcodeInvalidRequest = -32600
	errcodeMethodNotFound = -32601
	errcodeInvalidParams  = -32602
	errcodeInternal       = -32603
	errcodeDefault        = -32000
)

type jsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
}

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *jsonError) Error() string { return e.Message }

// DataError lets a business error attach structured fields to error.data.
type DataError interface {
	error
	ErrorCode() int
	ErrorData() interface{}
}

func errorResponse(id json.RawMessage, je *jsonError) *jsonrpcMessage {
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: id, Error: je}
}

func toJSONError(err error) *jsonError {
	if je, ok := err.(*jsonError); ok {
		return je
	}
	je := &jsonError{Code: errcodeDefault, Message: err.Error()}
	if de, ok := err.(DataError); ok {
		je.Code = de.ErrorCode()
		je.Data = de.ErrorData()
	}
	return je
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/jsonrpc/ -run TestToJSONError -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/jsonrpc/json.go api/jsonrpc/json_test.go
git commit -m "feat(jsonrpc): add JSON-RPC 2.0 envelope and error model"
```

---

### Task 2: Reflection registry core

**Files:**
- Create: `api/jsonrpc/service.go`
- Test: `api/jsonrpc/service_test.go`

**Interfaces:**
- Consumes: `jsonError`, `errcodeInvalidParams`, `errcodeInternal` (Task 1).
- Produces:
  - `type callback struct{ rcvr reflect.Value; fn reflect.Value; argTypes []reflect.Type; hasCtx bool; errPos int }`
  - `type serviceRegistry struct{ mu sync.Mutex; services map[string]map[string]*callback }`
  - `func (r *serviceRegistry) registerName(namespace string, rcvr interface{}) error`
  - `func (r *serviceRegistry) callback(method string) *callback`
  - `func (c *callback) parseArgs(rawParams json.RawMessage) ([]reflect.Value, error)`
  - `func (c *callback) call(ctx context.Context, args []reflect.Value) (interface{}, error)`
  - helpers `suitableCallbacks`, `newCallback`, `isErrorType`, `formatName`.

- [ ] **Step 1: Write the failing test**

Create `api/jsonrpc/service_test.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummy struct{}

func (dummy) Echo(s string) (string, error)                 { return s, nil }
func (dummy) WithCtx(ctx context.Context, n int) (int, error) { return n, nil }
func (dummy) NoReturn()                                      {}
func (dummy) Boom() (string, error)                          { panic("boom") }
func (dummy) Bad() (int, int, int)                           { return 0, 0, 0 } // >2 returns, skipped
func (dummy) internal()                                      {}                 // unexported, skipped

func TestRegisterNameValidation(t *testing.T) {
	var r serviceRegistry
	assert.Error(t, r.registerName("", dummy{}))          // empty namespace
	assert.Error(t, r.registerName("empty", struct{}{}))  // no suitable methods
	require.NoError(t, r.registerName("dummy", dummy{}))
}

func TestCallbackDiscoveryAndSignatures(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))

	assert.NotNil(t, r.callback("dummy_echo"))
	assert.NotNil(t, r.callback("dummy_noReturn"))
	assert.Nil(t, r.callback("dummy_bad"))      // >2 returns skipped
	assert.Nil(t, r.callback("dummy_internal")) // unexported skipped
	assert.Nil(t, r.callback("noUnderscore"))

	echo := r.callback("dummy_echo")
	assert.False(t, echo.hasCtx)
	assert.Len(t, echo.argTypes, 1)
	assert.Equal(t, 1, echo.errPos)

	withCtx := r.callback("dummy_withCtx")
	assert.True(t, withCtx.hasCtx)
	assert.Len(t, withCtx.argTypes, 1) // ctx not counted
}

func TestParseArgs(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))
	echo := r.callback("dummy_echo")

	// valid single positional arg
	args, err := echo.parseArgs(json.RawMessage(`["hi"]`))
	require.NoError(t, err)
	require.Len(t, args, 1)
	assert.Equal(t, "hi", args[0].String())

	// too many args -> -32602
	_, err = echo.parseArgs(json.RawMessage(`["a","b"]`))
	require.Error(t, err)
	assert.Equal(t, errcodeInvalidParams, err.(*jsonError).Code)

	// wrong type -> -32602
	_, err = echo.parseArgs(json.RawMessage(`[5]`))
	require.Error(t, err)
	assert.Equal(t, errcodeInvalidParams, err.(*jsonError).Code)

	// missing optional trailing arg -> zero value
	args, err = echo.parseArgs(json.RawMessage(`[]`))
	require.NoError(t, err)
	require.Len(t, args, 1)
	assert.Equal(t, "", args[0].String())
}

func TestCallResultErrorPanic(t *testing.T) {
	var r serviceRegistry
	require.NoError(t, r.registerName("dummy", dummy{}))
	ctx := context.Background()

	echo := r.callback("dummy_echo")
	args, _ := echo.parseArgs(json.RawMessage(`["yo"]`))
	res, err := echo.call(ctx, args)
	require.NoError(t, err)
	assert.Equal(t, "yo", res)

	no := r.callback("dummy_noReturn")
	res, err = no.call(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, res)

	boom := r.callback("dummy_boom")
	_, err = boom.call(ctx, nil)
	require.Error(t, err)
	assert.Equal(t, errcodeInternal, err.(*jsonError).Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/jsonrpc/ -run 'TestRegisterName|TestCallback|TestParseArgs|TestCall' -v`
Expected: FAIL — `undefined: serviceRegistry`.

- [ ] **Step 3: Write minimal implementation**

Create `api/jsonrpc/service.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

type callback struct {
	rcvr     reflect.Value
	fn       reflect.Value
	argTypes []reflect.Type
	hasCtx   bool
	errPos   int
}

type serviceRegistry struct {
	mu       sync.Mutex
	services map[string]map[string]*callback
}

func (r *serviceRegistry) registerName(namespace string, rcvr interface{}) error {
	if namespace == "" {
		return fmt.Errorf("jsonrpc: namespace cannot be empty")
	}
	callbacks := suitableCallbacks(reflect.ValueOf(rcvr))
	if len(callbacks) == 0 {
		return fmt.Errorf("jsonrpc: service %T has no suitable methods to expose", rcvr)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services == nil {
		r.services = make(map[string]map[string]*callback)
	}
	svc := r.services[namespace]
	if svc == nil {
		svc = make(map[string]*callback)
		r.services[namespace] = svc
	}
	for name, cb := range callbacks {
		svc[name] = cb
	}
	return nil
}

func (r *serviceRegistry) callback(method string) *callback {
	i := strings.IndexByte(method, '_')
	if i <= 0 {
		return nil
	}
	ns, name := method[:i], method[i+1:]
	r.mu.Lock()
	defer r.mu.Unlock()
	if svc := r.services[ns]; svc != nil {
		return svc[name]
	}
	return nil
}

func suitableCallbacks(receiver reflect.Value) map[string]*callback {
	typ := receiver.Type()
	out := make(map[string]*callback)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		if method.PkgPath != "" {
			continue
		}
		if cb := newCallback(receiver, method.Func); cb != nil {
			out[formatName(method.Name)] = cb
		}
	}
	return out
}

func newCallback(receiver, fn reflect.Value) *callback {
	fnType := fn.Type()

	errPos := -1
	switch fnType.NumOut() {
	case 0:
	case 1:
		if isErrorType(fnType.Out(0)) {
			errPos = 0
		}
	case 2:
		if !isErrorType(fnType.Out(1)) {
			return nil
		}
		errPos = 1
	default:
		return nil
	}

	hasCtx := false
	firstArg := 1
	if fnType.NumIn() > 1 && fnType.In(1) == contextType {
		hasCtx = true
		firstArg = 2
	}
	argTypes := make([]reflect.Type, 0, fnType.NumIn()-firstArg)
	for i := firstArg; i < fnType.NumIn(); i++ {
		argTypes = append(argTypes, fnType.In(i))
	}
	return &callback{rcvr: receiver, fn: fn, argTypes: argTypes, hasCtx: hasCtx, errPos: errPos}
}

func (c *callback) parseArgs(rawParams json.RawMessage) ([]reflect.Value, error) {
	if len(c.argTypes) == 0 {
		return nil, nil
	}
	var params []json.RawMessage
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &jsonError{Code: errcodeInvalidParams, Message: err.Error()}
		}
	}
	if len(params) > len(c.argTypes) {
		return nil, &jsonError{Code: errcodeInvalidParams,
			Message: fmt.Sprintf("too many arguments, want at most %d", len(c.argTypes))}
	}
	args := make([]reflect.Value, len(c.argTypes))
	for i, t := range c.argTypes {
		if i < len(params) && len(params[i]) > 0 && string(params[i]) != "null" {
			val := reflect.New(t)
			if err := json.Unmarshal(params[i], val.Interface()); err != nil {
				return nil, &jsonError{Code: errcodeInvalidParams,
					Message: fmt.Sprintf("invalid argument %d: %v", i, err)}
			}
			args[i] = val.Elem()
		} else {
			args[i] = reflect.Zero(t)
		}
	}
	return args, nil
}

func (c *callback) call(ctx context.Context, args []reflect.Value) (res interface{}, errRes error) {
	full := make([]reflect.Value, 0, 2+len(args))
	full = append(full, c.rcvr)
	if c.hasCtx {
		full = append(full, reflect.ValueOf(ctx))
	}
	full = append(full, args...)

	defer func() {
		if r := recover(); r != nil {
			errRes = &jsonError{Code: errcodeInternal, Message: "method handler crashed"}
		}
	}()

	results := c.fn.Call(full)
	if len(results) == 0 {
		return nil, nil
	}
	if c.errPos >= 0 && !results[c.errPos].IsNil() {
		return nil, results[c.errPos].Interface().(error)
	}
	if c.errPos == 0 {
		return nil, nil
	}
	return results[0].Interface(), nil
}

func isErrorType(t reflect.Type) bool { return t.Implements(errorType) }

func formatName(name string) string {
	r := []rune(name)
	if len(r) > 0 {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/jsonrpc/ -run 'TestRegisterName|TestCallback|TestParseArgs|TestCall' -v`
Expected: PASS (all 4 tests).

- [ ] **Step 5: Commit**

```bash
git add api/jsonrpc/service.go api/jsonrpc/service_test.go
git commit -m "feat(jsonrpc): add reflection-based service registry"
```

---

### Task 3: Server dispatch

**Files:**
- Create: `api/jsonrpc/server.go`
- Test: `api/jsonrpc/server_test.go`

**Interfaces:**
- Consumes: `serviceRegistry`, `callback` (Task 2); `jsonrpcMessage`, `jsonError`, `errorResponse`, `toJSONError`, error codes (Task 1); the `dummy` service type from `service_test.go` (same package).
- Produces:
  - `type Server struct{ registry serviceRegistry }`
  - `func NewServer() *Server`
  - `func (s *Server) RegisterName(namespace string, rcvr interface{}) error`
  - `func (s *Server) handleMsg(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage`

- [ ] **Step 1: Write the failing test**

Create `api/jsonrpc/server_test.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDummyServer(t *testing.T) *Server {
	srv := NewServer()
	require.NoError(t, srv.RegisterName("dummy", dummy{}))
	return srv
}

func TestHandleMsg(t *testing.T) {
	srv := newDummyServer(t)
	ctx := context.Background()

	// success
	resp := srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"),
		Method: "dummy_echo", Params: json.RawMessage(`["hi"]`)})
	assert.Nil(t, resp.Error)
	assert.Equal(t, "hi", resp.Result)

	// wrong version -> -32600
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "1.0", Method: "dummy_echo"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInvalidRequest, resp.Error.Code)

	// unknown method -> -32601
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", Method: "dummy_nope"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeMethodNotFound, resp.Error.Code)

	// bad params -> -32602
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", Method: "dummy_echo",
		Params: json.RawMessage(`[1,2]`)})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInvalidParams, resp.Error.Code)

	// panic -> -32603
	resp = srv.handleMsg(ctx, &jsonrpcMessage{Version: "2.0", Method: "dummy_boom"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, errcodeInternal, resp.Error.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/jsonrpc/ -run TestHandleMsg -v`
Expected: FAIL — `undefined: NewServer`.

- [ ] **Step 3: Write minimal implementation**

Create `api/jsonrpc/server.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "context"

type Server struct {
	registry serviceRegistry
}

func NewServer() *Server { return &Server{} }

func (s *Server) RegisterName(namespace string, rcvr interface{}) error {
	return s.registry.registerName(namespace, rcvr)
}

func (s *Server) handleMsg(ctx context.Context, msg *jsonrpcMessage) *jsonrpcMessage {
	if msg.Version != jsonrpcVersion || msg.Method == "" {
		return errorResponse(msg.ID, &jsonError{Code: errcodeInvalidRequest, Message: "invalid request"})
	}
	cb := s.registry.callback(msg.Method)
	if cb == nil {
		return errorResponse(msg.ID, &jsonError{Code: errcodeMethodNotFound,
			Message: "the method " + msg.Method + " does not exist/is not available"})
	}
	args, err := cb.parseArgs(msg.Params)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	result, err := cb.call(ctx, args)
	if err != nil {
		return errorResponse(msg.ID, toJSONError(err))
	}
	return &jsonrpcMessage{Version: jsonrpcVersion, ID: msg.ID, Result: result}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/jsonrpc/ -run TestHandleMsg -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/jsonrpc/server.go api/jsonrpc/server_test.go
git commit -m "feat(jsonrpc): add server dispatch for single messages"
```

---

### Task 4: Backend and example methods

**Files:**
- Create: `api/jsonrpc/backend.go`, `api/jsonrpc/eth.go`, `api/jsonrpc/net.go`, `api/jsonrpc/web3.go`
- Test: `api/jsonrpc/methods_test.go`

**Interfaces:**
- Consumes: `Server`, `NewServer`, `RegisterName`, `handleMsg` (Task 3); `jsonrpcMessage` (Task 1). thor: `chain.Repository`, `state.Stater`, `bft.Committer`, `thor.ForkConfig`, `restutil.ParseRevision`, `restutil.GetSummaryAndState`.
- Produces:
  - `type backend struct{ repo *chain.Repository; stater *state.Stater; bft bft.Committer; forkConfig *thor.ForkConfig }`
  - `func (b *backend) stateForRevision(revStr string) (*chain.BlockSummary, *state.State, error)`
  - `type ethAPI struct{ b *backend }` with `ChainId() (*hexutil.Big, error)`, `BlockNumber() (hexutil.Uint64, error)`, `GetBalance(ctx context.Context, addr common.Address, blockParam *string) (*hexutil.Big, error)`.
  - `type netAPI struct{ b *backend }` with `Version() string`.
  - `type web3API struct{}` with `ClientVersion() string`.

- [ ] **Step 1: Write the failing test**

Create `api/jsonrpc/methods_test.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
)

func newTestBackend(t *testing.T) *backend {
	tc, err := testchain.NewDefault()
	require.NoError(t, err)
	return &backend{
		repo:       tc.Repo(),
		stater:     tc.Stater(),
		bft:        tc.Engine(),
		forkConfig: tc.GetForkConfig(),
	}
}

func newMethodServer(t *testing.T) *Server {
	b := newTestBackend(t)
	srv := NewServer()
	require.NoError(t, srv.RegisterName("eth", &ethAPI{b: b}))
	require.NoError(t, srv.RegisterName("net", &netAPI{b: b}))
	require.NoError(t, srv.RegisterName("web3", &web3API{}))
	return srv
}

func dispatchJSON(t *testing.T, srv *Server, method string) string {
	resp := srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: method})
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	return string(out)
}

func TestExampleMethods(t *testing.T) {
	srv := newMethodServer(t)

	// genesis-only chain: best block number is 0 -> "0x0"
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"0x0"}`, dispatchJSON(t, srv, "eth_blockNumber"))

	// web3_clientVersion is a fixed string
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"thor"}`, dispatchJSON(t, srv, "web3_clientVersion"))

	// chainId and net_version are deterministic per genesis but value not asserted; must not error
	resp := srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: "eth_chainId"})
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	resp = srv.handleMsg(context.Background(),
		&jsonrpcMessage{Version: "2.0", ID: json.RawMessage("1"), Method: "net_version"})
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/jsonrpc/ -run TestExampleMethods -v`
Expected: FAIL — `undefined: backend` / `ethAPI`.

- [ ] **Step 3: Write minimal implementation**

Create `api/jsonrpc/backend.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type backend struct {
	repo       *chain.Repository
	stater     *state.Stater
	bft        bft.Committer
	forkConfig *thor.ForkConfig
}

// stateForRevision reuses the REST revision resolver so JSON-RPC and REST see the
// same chain state. GetSummaryAndState takes a *restutil.Revision, so parse first.
func (b *backend) stateForRevision(revStr string) (*chain.BlockSummary, *state.State, error) {
	rev, err := restutil.ParseRevision(revStr, false)
	if err != nil {
		return nil, nil, err
	}
	return restutil.GetSummaryAndState(rev, b.repo, b.bft, b.stater, b.forkConfig)
}
```

Create `api/jsonrpc/eth.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
)

type ethAPI struct{ b *backend }

// eth_chainId
func (a *ethAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(new(big.Int).SetUint64(a.b.repo.ChainID())), nil
}

// eth_blockNumber
func (a *ethAPI) BlockNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(a.b.repo.BestBlockSummary().Header.Number()), nil
}

// eth_getBalance. Bootstrap: blockParam empty/"latest" => best; other values are
// forwarded verbatim to ParseRevision. Full BlockNumberOrHash union + ethereum<->thor
// revision mapping is deferred to Phase 1.
func (a *ethAPI) GetBalance(ctx context.Context, addr common.Address, blockParam *string) (*hexutil.Big, error) {
	rev := "best"
	if blockParam != nil && *blockParam != "" && *blockParam != "latest" {
		rev = *blockParam
	}
	_, st, err := a.b.stateForRevision(rev)
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	bal, err := st.GetBalance(thor.Address(addr))
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	return (*hexutil.Big)(bal), nil
}
```

Create `api/jsonrpc/net.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import "strconv"

type netAPI struct{ b *backend }

// net_version
func (a *netAPI) Version() string {
	return strconv.FormatUint(a.b.repo.ChainID(), 10)
}
```

Create `api/jsonrpc/web3.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

type web3API struct{}

// web3_clientVersion
func (a *web3API) ClientVersion() string {
	return "thor"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/jsonrpc/ -run TestExampleMethods -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/jsonrpc/backend.go api/jsonrpc/eth.go api/jsonrpc/net.go api/jsonrpc/web3.go api/jsonrpc/methods_test.go
git commit -m "feat(jsonrpc): add backend and eth/net/web3 example methods"
```

---

### Task 5: HTTP entry point and Mount

**Files:**
- Create: `api/jsonrpc/jsonrpc.go`
- Test: `api/jsonrpc/http_test.go`

**Interfaces:**
- Consumes: `Server`, `NewServer`, `RegisterName`, `handleMsg` (Task 3); `backend`, `ethAPI`, `netAPI`, `web3API` (Task 4); `jsonrpcMessage`, `errorResponse`, `jsonError`, `errcodeParse` (Task 1). thor: `restutil.WrapHandlerFunc`, `restutil.WriteJSON`, `chain.Repository`, `state.Stater`, `bft.Committer`, `thor.ForkConfig`, `gorilla/mux`.
- Produces:
  - `type JSONRPC struct{ server *Server }`
  - `func New(repo *chain.Repository, stater *state.Stater, bft bft.Committer, forkConfig *thor.ForkConfig) *JSONRPC`
  - `func (j *JSONRPC) Mount(root *mux.Router, pathPrefix string)`
  - `func (j *JSONRPC) handleHTTP(w http.ResponseWriter, r *http.Request) error`

- [ ] **Step 1: Write the failing test**

Create `api/jsonrpc/http_test.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
)

func newHTTPServer(t *testing.T) *httptest.Server {
	tc, err := testchain.NewDefault()
	require.NoError(t, err)
	router := mux.NewRouter()
	New(tc.Repo(), tc.Stater(), tc.Engine(), tc.GetForkConfig()).Mount(router, "/rpc")
	return httptest.NewServer(router)
}

func post(t *testing.T, url, body string) string {
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(out)
}

func TestHTTPSingleAndBatch(t *testing.T) {
	ts := newHTTPServer(t)
	defer ts.Close()
	url := ts.URL + "/rpc"

	// single: blockNumber on genesis-only chain
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":"0x0"}`,
		post(t, url, `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`))

	// getBalance of the zero address => 0
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":2,"result":"0x0"}`,
		post(t, url, `{"jsonrpc":"2.0","id":2,"method":"eth_getBalance","params":["0x0000000000000000000000000000000000000000"]}`))

	// batch of two
	assert.JSONEq(t, `[{"jsonrpc":"2.0","id":1,"result":"0x0"},{"jsonrpc":"2.0","id":2,"result":"thor"}]`,
		post(t, url, `[{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"},{"jsonrpc":"2.0","id":2,"method":"web3_clientVersion"}]`))

	// unknown method -> -32601
	resp := post(t, url, `{"jsonrpc":"2.0","id":9,"method":"eth_nope"}`)
	assert.Contains(t, resp, "-32601")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/jsonrpc/ -run TestHTTPSingleAndBatch -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

Create `api/jsonrpc/jsonrpc.go`:
```go
// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type JSONRPC struct {
	server *Server
}

func New(repo *chain.Repository, stater *state.Stater, bft bft.Committer, forkConfig *thor.ForkConfig) *JSONRPC {
	srv := NewServer()
	b := &backend{repo: repo, stater: stater, bft: bft, forkConfig: forkConfig}

	_ = srv.RegisterName("eth", &ethAPI{b: b})
	_ = srv.RegisterName("net", &netAPI{b: b})
	_ = srv.RegisterName("web3", &web3API{})

	return &JSONRPC{server: srv}
}

func (j *JSONRPC) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").Methods(http.MethodPost).Name("POST /rpc").
		HandlerFunc(restutil.WrapHandlerFunc(j.handleHTTP))
}

func (j *JSONRPC) handleHTTP(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	ctx := r.Context()
	trimmed := bytes.TrimLeft(body, " \t\r\n")

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var msgs []jsonrpcMessage
		if err := json.Unmarshal(body, &msgs); err != nil || len(msgs) == 0 {
			return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: "invalid batch"}))
		}
		resps := make([]*jsonrpcMessage, len(msgs))
		for i := range msgs {
			resps[i] = j.server.handleMsg(ctx, &msgs[i])
		}
		return restutil.WriteJSON(w, resps)
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	return restutil.WriteJSON(w, j.server.handleMsg(ctx, &msg))
}
```

- [ ] **Step 4: Run the full package test suite**

Run: `go test ./api/jsonrpc/ -v`
Expected: PASS — every test from Tasks 1–5.

- [ ] **Step 5: Commit**

```bash
git add api/jsonrpc/jsonrpc.go api/jsonrpc/http_test.go
git commit -m "feat(jsonrpc): add HTTP transport and /rpc mount"
```

---

### Task 6: Wire JSON-RPC into the node

**Files:**
- Modify: `cmd/thor/httpserver/api_server.go` (add `EnableRPC bool` to `APIConfig`; add guarded mount after the `subscriptions` mount, ~line 132)
- Modify: `cmd/thor/flags.go` (add `enableRPCFlag`, ~after `enableAPILogsFlag` near line 114)
- Modify: `cmd/thor/utils.go` (add `EnableRPC` field in `makeAPIConfig`, ~line 261)
- Modify: `cmd/thor/main.go` (register `enableRPCFlag` in both `Flags` slices, ~lines 108 and 154)

**Interfaces:**
- Consumes: `jsonrpc.New(repo, stater, bft, forkConfig).Mount(router, "/rpc")` (Task 5); existing `repo`, `stater`, `bft`, `forkConfig` locals in `StartAPIServer`.
- Produces: `--enable-rpc` CLI flag (default false, env `ENABLE_RPC`); `APIConfig.EnableRPC bool`.

- [ ] **Step 1: Add the config field and guarded mount**

In `cmd/thor/httpserver/api_server.go`, add to the import block:
```go
	"github.com/vechain/thor/v2/api/jsonrpc"
```

Add a field to `APIConfig` (after `Log5XXErrors bool`):
```go
	EnableRPC bool
```

After the subscriptions mount (`subs.Mount(router, "/subscriptions")`, ~line 132), add:
```go
	if config.EnableRPC {
		jsonrpc.New(repo, stater, bft, forkConfig).Mount(router, "/rpc")
	}
```

- [ ] **Step 2: Add the CLI flag**

In `cmd/thor/flags.go`, after the `enableAPILogsFlag` definition:
```go
	enableRPCFlag = &cli.BoolFlag{
		Name:    "enable-rpc",
		Local:   true,
		Usage:   "enables the eth-compatible JSON-RPC server under /rpc",
		Sources: envVar("ENABLE_RPC"),
	}
```

- [ ] **Step 3: Populate the field from the flag**

In `cmd/thor/utils.go`, inside the `httpserver.APIConfig{...}` literal in `makeAPIConfig`, add:
```go
		EnableRPC: ctx.Bool(enableRPCFlag.Name),
```

- [ ] **Step 4: Register the flag in both command flag slices**

In `cmd/thor/main.go`, add `enableRPCFlag,` to the `Flags: []cli.Flag{...}` slice at ~line 91 (the node `run` command, next to `enableAPILogsFlag,`) AND to the `Flags: []cli.Flag{...}` slice at ~line 139 (the `solo` command, next to `enableAPILogsFlag,`).

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./api/jsonrpc/... ./cmd/thor/...`
Expected: exit 0, no output.

- [ ] **Step 6: Manual end-to-end check**

Run:
```bash
make
./bin/thor solo --on-demand --enable-rpc &
sleep 3
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}'
curl -s -X POST http://127.0.0.1:8669/rpc -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"web3_clientVersion"}'
kill %1
```
Expected: first returns `{"jsonrpc":"2.0","id":1,"result":"0x..."}`; second returns `{"jsonrpc":"2.0","id":2,"result":"thor"}`.

- [ ] **Step 7: Commit**

```bash
git add cmd/thor/httpserver/api_server.go cmd/thor/flags.go cmd/thor/utils.go cmd/thor/main.go
git commit -m "feat(jsonrpc): wire JSON-RPC server into node behind --enable-rpc"
```

---

## Self-Review Notes

- **Spec coverage:** §5.2 → Task 2; §5.3 → Task 1; §5.4 → Task 3; §5.5 → Task 5; §5.6 → Task 4; §5.7 + §5.5 mount diff → Task 6; §7 verification → Task 5 Step 4 + Task 6 Steps 5–6. §2.5 (middleware side effects) is analysis with no bootstrap code change; nothing to implement. §1.3 Non-goals are intentionally excluded.
- **Type consistency:** `backend` fields (`repo`/`stater`/`bft`/`forkConfig`) match `New`'s parameter order and `stateForRevision`'s use; `handleMsg` signature identical across Tasks 3–5; `restutil.GetSummaryAndState(rev *Revision, ...)` consumed via `ParseRevision` in Task 4.
- **Placeholders:** none — every code and test step is complete. The single `TODO`-like note in `eth.go` (`GetBalance` block-param) is an intentional, documented scope boundary matching spec §1.3, not an unfinished step.
