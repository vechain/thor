// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/log"
)

// mockLogger is a simple logger implementation for testing purposes
type mockLogger struct {
	loggedData []any
}

func (m *mockLogger) With(_ ...any) log.Logger {
	return m
}

func (m *mockLogger) Log(_ slog.Level, _ string, _ ...any) {}

func (m *mockLogger) Trace(_ string, _ ...any) {}

func (m *mockLogger) Write(_ slog.Level, _ string, _ ...any) {}

func (m *mockLogger) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (m *mockLogger) Handler() slog.Handler { return nil }

func (m *mockLogger) New(_ ...any) log.Logger { return m }

func (m *mockLogger) Debug(_ string, _ ...any) {}

func (m *mockLogger) Error(_ string, _ ...any) {}

func (m *mockLogger) Crit(_ string, _ ...any) {}

func (m *mockLogger) Info(_ string, ctx ...any) {
	m.loggedData = append(m.loggedData, ctx...)
}

func (m *mockLogger) Warn(_ string, ctx ...any) {
	m.loggedData = append(m.loggedData, ctx...)
}

func (m *mockLogger) GetLoggedData() []any {
	return m.loggedData
}

func TestRequestLoggerHandler(t *testing.T) {
	mockLog := &mockLogger{}
	enabled := atomic.Bool{}
	enabled.Store(true)

	// Define a test handler to wrap
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create the RequestLoggerHandler
	loggerHandler := RequestLoggerHandler(testHandler, mockLog, &enabled)

	// Create a test HTTP request
	reqBody := "test body"
	req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Serve the HTTP request
	loggerHandler.ServeHTTP(rr, req)

	// Check the response status code
	assert.Equal(t, http.StatusOK, rr.Code)

	// Check the response body
	assert.Equal(t, "OK", rr.Body.String())

	// Verify that the logger recorded the correct information
	loggedData := mockLog.GetLoggedData()
	assert.Contains(t, loggedData, "URI")
	assert.Contains(t, loggedData, "http://example.com/foo")
	assert.Contains(t, loggedData, "Method")
	assert.Contains(t, loggedData, "POST")
	assert.Contains(t, loggedData, "Body")
	assert.Contains(t, loggedData, reqBody)

	// Check if timestamp is present
	foundTimestamp := false
	for i := 0; i < len(loggedData); i += 2 {
		if loggedData[i] == "timestamp" {
			_, ok := loggedData[i+1].(int64)
			assert.True(t, ok, "timestamp should be an int64")
			foundTimestamp = true
			break
		}
	}
	assert.True(t, foundTimestamp, "timestamp should be logged")
}
