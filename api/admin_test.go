// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/log"
)

func TestAdmin_postLogLevel(t *testing.T) {
	tests := []struct {
		level    string
		httpCode int
	}{
		{"debug", http.StatusOK},
		{"info", http.StatusOK},
		{"warn", http.StatusOK},
		{"error", http.StatusOK},
		{"crit", http.StatusOK},
		{"invalid", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			admin := newAdmin()
			req := newRequest(t, http.MethodPost, "/admin/loglevel", map[string]string{"level": tt.level})
			res := newHTTPTest(req, admin.postLogLevelHandler)

			assert.Equal(t, tt.httpCode, res.Code)
			if tt.httpCode == http.StatusOK {
				assert.Equal(t, tt.level, log.LevelString(admin.logLevel.Level()))
			}
		})
	}
}

func TestAdmin_getLogLevel(t *testing.T) {
	admin := newAdmin()
	initialLevel := log.LevelString(admin.logLevel.Level())
	req := newRequest(t, http.MethodGet, "/admin/loglevel", nil)

	res := newHTTPTest(req, admin.getRequestLoggerEnabled)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, initialLevel, log.LevelString(admin.logLevel.Level()))
}

// Update TestAdmin_postRequestLogger
func TestAdmin_postRequestLogger(t *testing.T) {
	testCases := []struct {
		enabled  interface{}
		httpCode int
	}{
		{true, http.StatusOK},
		{false, http.StatusOK},
		{"invalid", http.StatusBadRequest},
		{nil, http.StatusBadRequest},
	}

	for _, tt := range testCases {
		t.Run(fmt.Sprintf("enabled=%v", tt.enabled), func(t *testing.T) {
			admin := newAdmin()
			req := newRequest(t, http.MethodPost, "/admin/apilogs", map[string]interface{}{"enabled": tt.enabled})

			res := newHTTPTest(req, admin.postRequestLogger)

			assert.Equal(t, tt.httpCode, res.Code)
			if res.Code == http.StatusOK {
				assert.Equal(t, tt.enabled, admin.logRequests.Load())
			}
		})
	}
}

// Update TestAdmin_getRequestLoggerEnabled
func TestAdmin_getRequestLoggerEnabled(t *testing.T) {
	admin := newAdmin()
	req := newRequest(t, http.MethodGet, "/admin/apilogs", nil)

	res := newHTTPTest(req, admin.getRequestLoggerEnabled)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.True(t, admin.logRequests.Load())
}

func newHTTPTest(req *http.Request, handlerFunc utils.HandlerFunc) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler := utils.WrapHandlerFunc(handlerFunc)
	handler.ServeHTTP(rr, req)
	return rr
}

func newAdmin() *Admin {
	var lvl slog.LevelVar
	lvl.Set(slog.LevelDebug)

	var enabled atomic.Bool
	enabled.Store(true)

	return NewAdmin("localhost:0", &lvl, &enabled)
}

func newRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	reqBody := marshalBody(t, body)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func marshalBody(t *testing.T, body interface{}) []byte {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("could not marshal request body: %v", err)
		}
	}
	return reqBody
}
