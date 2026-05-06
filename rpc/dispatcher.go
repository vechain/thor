// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import "fmt"

// Dispatcher routes JSON-RPC requests to registered method handlers.
// Sub-packages call Register via their Mount method; the Server calls dispatch.
type Dispatcher struct {
	methods map[string]func(Request) Response
}

// NewDispatcher creates an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{methods: make(map[string]func(Request) Response)}
}

// Register adds a handler for the given JSON-RPC method name.
// Panics if the method name is already registered — catches wiring mistakes at startup.
func (d *Dispatcher) Register(method string, handler func(Request) Response) {
	if _, exists := d.methods[method]; exists {
		panic("rpc: duplicate method registration: " + method)
	}
	d.methods[method] = handler
}

func (d *Dispatcher) dispatch(req Request) Response {
	h, ok := d.methods[req.Method]
	if !ok {
		return ErrResponse(req.ID, CodeMethodNotFound, fmt.Sprintf("method %q not found", req.Method))
	}
	return h(req)
}
