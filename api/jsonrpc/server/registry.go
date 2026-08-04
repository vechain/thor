// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"unicode"

	"github.com/vechain/thor/v2/log"
)

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
	logger      = log.WithContext("pkg", "jsonrpc")
)

type callback struct {
	rcvr     reflect.Value
	fn       reflect.Value
	argTypes []reflect.Type
	hasCtx   bool
	errPos   int
}

// registry maps namespace -> method name -> callback. See Server.RegisterName
// for the registration contract (signatures, ctx handling, naming).
type registry struct {
	mu       sync.Mutex
	services map[string]map[string]*callback
}

// registerNamespace backs Server.RegisterName; see it for the full contract.
func (r *registry) registerNamespace(namespace string, rcvr any) error {
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
	for name := range callbacks {
		if _, exists := svc[name]; exists {
			return fmt.Errorf("jsonrpc: method %q already registered in namespace %q", name, namespace)
		}
	}
	maps.Copy(svc, callbacks)
	return nil
}

func (r *registry) callback(method string) *callback {
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
	for method := range typ.Methods() {
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
		return nil, &jsonError{
			Code:    errcodeInvalidParams,
			Message: fmt.Sprintf("too many arguments, want at most %d", len(c.argTypes)),
		}
	}
	args := make([]reflect.Value, len(c.argTypes))
	for i, t := range c.argTypes {
		if i >= len(params) {
			// geth convention: pointer args are optional, non-pointer required.
			if t.Kind() != reflect.Pointer {
				return nil, &jsonError{
					Code:    errcodeInvalidParams,
					Message: fmt.Sprintf("missing value for required argument %d", i),
				}
			}
			args[i] = reflect.Zero(t)
			continue
		}
		val := reflect.New(t)
		if err := json.Unmarshal(params[i], val.Interface()); err != nil {
			return nil, &jsonError{
				Code:    errcodeInvalidParams,
				Message: fmt.Sprintf("invalid argument %d: %v", i, err),
			}
		}
		args[i] = val.Elem()
	}
	return args, nil
}

func (c *callback) call(ctx context.Context, args []reflect.Value) (res any, errRes error) {
	full := make([]reflect.Value, 0, 2+len(args))
	full = append(full, c.rcvr)
	if c.hasCtx {
		full = append(full, reflect.ValueOf(ctx))
	}
	full = append(full, args...)

	defer func() {
		if r := recover(); r != nil {
			logger.Error("jsonrpc method panicked", "recover", r, "stack", string(debug.Stack()))
			errRes = &jsonError{Code: errcodeInternal, Message: "internal server error"}
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
