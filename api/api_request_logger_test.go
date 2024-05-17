// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inconshreveable/log15"
	"github.com/stretchr/testify/assert"
)

// mockLogger is a simple logger implementation for testing purposes
type mockLogger struct {
	loggedData []interface{}
}

func (m *mockLogger) New(ctx ...interface{}) log15.Logger { return m }

func (m *mockLogger) GetHandler() log15.Handler { return nil }

func (m *mockLogger) SetHandler(h log15.Handler) {}

func (m *mockLogger) Debug(msg string, ctx ...interface{}) {}

func (m *mockLogger) Error(msg string, ctx ...interface{}) {}

func (m *mockLogger) Crit(msg string, ctx ...interface{}) {}

func (m *mockLogger) Info(msg string, ctx ...interface{}) {
	m.loggedData = append(m.loggedData, ctx...)
}

func (m *mockLogger) Warn(msg string, ctx ...interface{}) {
	m.loggedData = append(m.loggedData, ctx...)
}

func (m *mockLogger) GetLoggedData() []interface{} {
	return m.loggedData
}

func TestRequestLoggerHandler(t *testing.T) {
	mockLog := &mockLogger{}

	// Define a test handler to wrap
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create the RequestLoggerHandler
	loggerHandler := RequestLoggerHandler(testHandler, mockLog)

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
