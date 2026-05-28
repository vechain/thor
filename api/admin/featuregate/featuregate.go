// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package featuregate provides a generic runtime on/off toggle backed by
// an *atomic.Bool. Each Gate exposes a Middleware that returns 503 when
// disabled, and an optional TTL after which the gate auto-disables.
// A Registry groups gates under a single REST namespace
// (e.g. /admin/features/{name}) and can also expose per-feature legacy URLs
// as aliases for backward compatibility.
package featuregate

import (
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/log"
)

const maxTTLSeconds = 3600

// Observer is invoked on every toggle change (manual or TTL-triggered).
// May be nil.
type Observer func(name string, enabled bool)

// Gate is a runtime on/off switch.
type Gate struct {
	name     string
	enabled  *atomic.Bool
	observer Observer
	mu       sync.Mutex
	ttlTimer *time.Timer
	// gen is bumped on every Set under mu. Each TTL timer callback captures
	// its own birth-gen and bails if it has been superseded — this closes
	// the race where Timer.Stop misses an already-dispatched callback.
	gen uint64
}

// New returns a Gate backed by the provided atomic.Bool. The bool is owned
// by the caller (typically created in main and shared between admin and
// business servers) so its lifecycle is independent of the Gate.
// observer may be nil.
func New(name string, enabled *atomic.Bool, observer Observer) *Gate {
	return &Gate{name: name, enabled: enabled, observer: observer}
}

func (g *Gate) record(enabled bool) {
	if g.observer != nil {
		g.observer(g.name, enabled)
	}
}

func (g *Gate) Name() string  { return g.name }
func (g *Gate) Enabled() bool { return g.enabled.Load() }

// Middleware returns 503 + Retry-After while the gate is disabled.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.enabled.Load() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, g.name+" is disabled", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Set atomically toggles the gate, replaces any pending TTL timer, and
// records audit metrics. ttlSeconds is clamped to [0, maxTTLSeconds]; a
// non-zero TTL only takes effect when enabled is true.
func (g *Gate) Set(enabled bool, ttlSeconds int) api.ToggleStatus {
	g.mu.Lock()

	if ttlSeconds < 0 {
		ttlSeconds = 0
	} else if ttlSeconds > maxTTLSeconds {
		ttlSeconds = maxTTLSeconds
	}

	if g.ttlTimer != nil {
		g.ttlTimer.Stop()
		g.ttlTimer = nil
	}
	g.gen++
	myGen := g.gen
	g.enabled.Store(enabled)
	if enabled && ttlSeconds > 0 {
		g.ttlTimer = time.AfterFunc(time.Duration(ttlSeconds)*time.Second, func() {
			g.mu.Lock()
			if g.gen != myGen {
				// Superseded by a later Set that raced with our dispatch.
				g.mu.Unlock()
				return
			}
			g.ttlTimer = nil
			g.mu.Unlock()
			g.enabled.Store(false)
			g.record(false)
			log.Info(g.name+" auto-disabled after ttl", "pkg", "featuregate")
		})
	}
	g.mu.Unlock()

	// Run observer and logging outside the lock so a slow Observer can't
	// stall other callers.
	g.record(enabled)
	log.Info(g.name+" toggled", "pkg", "featuregate", "enabled", enabled, "ttlSeconds", ttlSeconds)
	return api.ToggleStatus{Enabled: g.enabled.Load(), TTLSeconds: ttlSeconds}
}

// Status returns the current enabled state without TTL info.
func (g *Gate) Status() api.ToggleStatus {
	return api.ToggleStatus{Enabled: g.enabled.Load()}
}

// Registry catalogs named Gates.
type Registry struct {
	mu    sync.Mutex
	gates map[string]*Gate
}

func NewRegistry() *Registry {
	return &Registry{gates: map[string]*Gate{}}
}

// Add registers g and returns it. Panics if a gate with the same name is
// already registered — Registry is expected to be wired once at startup
// and silent overwrite would mask a programmer error.
func (r *Registry) Add(g *Gate) *Gate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.gates[g.name]; exists {
		panic("featuregate: duplicate gate name: " + g.name)
	}
	r.gates[g.name] = g
	return g
}

// Get returns the named gate.
func (r *Registry) Get(name string) (*Gate, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.gates[name]
	return g, ok
}

// MountAPI mounts the unified REST endpoints under pathPrefix:
//
//	GET  pathPrefix         -> list all
//	GET  pathPrefix/{name}  -> single gate status
//	POST pathPrefix/{name}  -> toggle (body: api.ToggleStatus)
func (r *Registry) MountAPI(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").
		Methods(http.MethodGet).
		Name("list-features").
		HandlerFunc(restutil.WrapHandlerFunc(r.handleList))
	sub.Path("/{name}").
		Methods(http.MethodGet).
		Name("get-feature").
		HandlerFunc(restutil.WrapHandlerFunc(r.handleGet))
	sub.Path("/{name}").
		Methods(http.MethodPost).
		Name("post-feature").
		HandlerFunc(restutil.WrapHandlerFunc(r.handleSet))
}

// MountLegacyAlias mounts GET/POST at pathPrefix that operate on the named
// gate. Adds a Deprecation response header pointing at the unified URL.
func (r *Registry) MountLegacyAlias(root *mux.Router, pathPrefix, name string) {
	g, ok := r.Get(name)
	if !ok {
		panic("featuregate: legacy alias for unknown gate: " + name)
	}
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Use(deprecationHeader("/admin/features/" + name))
	sub.Path("").
		Methods(http.MethodGet).
		Name("legacy-get-" + name).
		HandlerFunc(restutil.WrapHandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
			return restutil.WriteJSON(w, g.Status())
		}))
	sub.Path("").
		Methods(http.MethodPost).
		Name("legacy-post-" + name).
		HandlerFunc(restutil.WrapHandlerFunc(func(w http.ResponseWriter, req *http.Request) error {
			return handleSetGate(w, req, g)
		}))
}

func (r *Registry) handleList(w http.ResponseWriter, _ *http.Request) error {
	r.mu.Lock()
	out := make([]namedStatus, 0, len(r.gates))
	for name, g := range r.gates {
		out = append(out, namedStatus{Name: name, Enabled: g.Enabled()})
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return restutil.WriteJSON(w, out)
}

func (r *Registry) handleGet(w http.ResponseWriter, req *http.Request) error {
	g, err := r.resolve(req)
	if err != nil {
		return err
	}
	return restutil.WriteJSON(w, g.Status())
}

func (r *Registry) handleSet(w http.ResponseWriter, req *http.Request) error {
	g, err := r.resolve(req)
	if err != nil {
		return err
	}
	return handleSetGate(w, req, g)
}

func (r *Registry) resolve(req *http.Request) (*Gate, error) {
	name := mux.Vars(req)["name"]
	g, ok := r.Get(name)
	if !ok {
		return nil, restutil.HTTPError(errors.New("unknown feature: "+name), http.StatusNotFound)
	}
	return g, nil
}

func handleSetGate(w http.ResponseWriter, req *http.Request, g *Gate) error {
	var body api.ToggleStatus
	if err := restutil.ParseJSON(req.Body, &body); err != nil {
		return restutil.BadRequest(err)
	}
	return restutil.WriteJSON(w, g.Set(body.Enabled, body.TTLSeconds))
}

func deprecationHeader(replacement string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Link", "<"+replacement+">; rel=\"successor-version\"")
			next.ServeHTTP(w, r)
		})
	}
}

type namedStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}
