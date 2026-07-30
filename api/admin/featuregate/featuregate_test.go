// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package featuregate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
)

func newRegistryRouter(gates map[string]bool) (*mux.Router, *Registry, map[string]*atomic.Bool) {
	reg := NewRegistry()
	flags := map[string]*atomic.Bool{}
	for name, initial := range gates {
		b := &atomic.Bool{}
		b.Store(initial)
		flags[name] = b
		reg.Add(New(name, b, nil))
	}
	router := mux.NewRouter()
	reg.MountAPI(router, "/admin/features")
	return router, reg, flags
}

func TestGateMiddleware503WhenDisabled(t *testing.T) {
	b := &atomic.Bool{}
	g := New("x", b, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rr := httptest.NewRecorder()
	g.Middleware(inner).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.NotEmpty(t, rr.Header().Get("Retry-After"))
}

func TestGateMiddlewareOKWhenEnabled(t *testing.T) {
	b := &atomic.Bool{}
	b.Store(true)
	g := New("x", b, nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rr := httptest.NewRecorder()
	g.Middleware(inner).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRegistryList(t *testing.T) {
	router, _, _ := newRegistryRouter(map[string]bool{"a": true, "b": false})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/features", nil))
	assert.Equal(t, http.StatusOK, rr.Code)

	var out []namedStatus
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	got := map[string]bool{}
	for _, s := range out {
		got[s.Name] = s.Enabled
	}
	assert.Equal(t, map[string]bool{"a": true, "b": false}, got)
}

func TestRegistryGetOne(t *testing.T) {
	router, _, _ := newRegistryRouter(map[string]bool{"a": true})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/features/a", nil))
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp api.ToggleStatus
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Enabled)
}

func TestRegistryGetUnknown(t *testing.T) {
	router, _, _ := newRegistryRouter(map[string]bool{"a": true})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/features/missing", nil))
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestRegistryPostFlip(t *testing.T) {
	router, _, flags := newRegistryRouter(map[string]bool{"a": false})
	body, _ := json.Marshal(api.ToggleStatus{Enabled: true})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/features/a", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, flags["a"].Load())
}

func TestTTLAutoDisable(t *testing.T) {
	b := &atomic.Bool{}
	g := New("x", b, nil)
	g.Set(true, 1)
	assert.True(t, b.Load())
	require.Eventually(t, func() bool { return !b.Load() }, 3*time.Second, 50*time.Millisecond)
}

func TestTTLClamp(t *testing.T) {
	b := &atomic.Bool{}
	g := New("x", b, nil)
	st := g.Set(true, 99999)
	assert.Equal(t, maxTTLSeconds, st.TTLSeconds)
	g.Set(false, 0) // stop timer
}

func TestSecondSetCancelsTimer(t *testing.T) {
	b := &atomic.Bool{}
	g := New("x", b, nil)
	g.Set(true, 1)
	g.Set(true, 0) // no TTL, should cancel previous timer
	time.Sleep(1500 * time.Millisecond)
	assert.True(t, b.Load(), "expected gate still enabled after old timer fired")
}

// TestSetWhileTimerFiring covers the race where a TTL timer has already
// dispatched its callback by the time a second Set tries to Stop it. Without
// the generation-counter guard the late callback would flip enabled back to
// false even though the latest user intent is "stay enabled".
func TestSetWhileTimerFiring(t *testing.T) {
	b := &atomic.Bool{}
	g := New("x", b, nil)
	g.Set(true, 1)
	// Sleep just under the TTL so the timer is about to dispatch (or has
	// already dispatched and is racing with our next Set call).
	time.Sleep(990 * time.Millisecond)
	g.Set(true, 0)
	time.Sleep(200 * time.Millisecond)
	assert.True(t, b.Load(), "second Set without TTL must win over a racing old timer")
}

func TestLegacyAlias(t *testing.T) {
	router := mux.NewRouter()
	reg := NewRegistry()
	b := &atomic.Bool{}
	reg.Add(New("a", b, nil))
	reg.MountLegacyAlias(router, "/admin/a", "a")

	body, _ := json.Marshal(api.ToggleStatus{Enabled: true})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/a", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, b.Load())
	assert.Equal(t, "true", rr.Header().Get("Deprecation"))
	assert.Contains(t, rr.Header().Get("Link"), "/admin/features/a")
}
