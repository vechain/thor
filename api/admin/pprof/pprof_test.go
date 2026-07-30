// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pprof

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/api/admin/featuregate"
)

func TestMountHandlersGatedByDefault(t *testing.T) {
	router := mux.NewRouter()
	flag := &atomic.Bool{}
	MountHandlers(router, featuregate.New("pprof", flag, nil))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestMountHandlersOKWhenEnabled(t *testing.T) {
	router := mux.NewRouter()
	flag := &atomic.Bool{}
	flag.Store(true)
	MountHandlers(router, featuregate.New("pprof", flag, nil))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil))
	assert.Equal(t, http.StatusOK, rr.Code)
}
