// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	tests := []struct {
		name                 string
		handler              http.HandlerFunc
		enabled              bool
		slowQueriesThreshold time.Duration
		log5xxErrors         bool
		expectedStatusCode   int
		shouldLog            bool
		description          string
	}{
		{
			name: "all logging enabled - fast 2xx response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			enabled:              true,
			slowQueriesThreshold: 0,
			log5xxErrors:         false,
			expectedStatusCode:   http.StatusOK,
			shouldLog:            true,
			description:          "should log when all logging is enabled",
		},
		{
			name: "all logging disabled - fast 2xx response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         false,
			expectedStatusCode:   http.StatusOK,
			shouldLog:            false,
			description:          "should not log when all logging is disabled",
		},
		{
			name: "slow query detection - request exceeds threshold",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(15 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			enabled:              false,
			slowQueriesThreshold: 10 * time.Millisecond,
			log5xxErrors:         false,
			expectedStatusCode:   http.StatusOK,
			shouldLog:            true,
			description:          "should log slow queries even when general logging is disabled",
		},
		{
			name: "slow query detection - request under threshold",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(5 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			enabled:              false,
			slowQueriesThreshold: 20 * time.Millisecond,
			log5xxErrors:         false,
			expectedStatusCode:   http.StatusOK,
			shouldLog:            false,
			description:          "should not log fast queries when under threshold",
		},
		{
			name: "5xx error logging - internal server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         true,
			expectedStatusCode:   http.StatusInternalServerError,
			shouldLog:            true,
			description:          "should log 500 errors when 5xx logging is enabled",
		},
		{
			name: "5xx error logging - service unavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service Unavailable"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         true,
			expectedStatusCode:   http.StatusServiceUnavailable,
			shouldLog:            true,
			description:          "should log 503 errors when 5xx logging is enabled",
		},
		{
			name: "5xx error logging disabled - internal server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         false,
			expectedStatusCode:   http.StatusInternalServerError,
			shouldLog:            false,
			description:          "should not log 5xx errors when 5xx logging is disabled",
		},
		{
			name: "4xx errors not logged - bad request",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         true,
			expectedStatusCode:   http.StatusBadRequest,
			shouldLog:            false,
			description:          "should not log 4xx errors even when 5xx logging is enabled",
		},
		{
			name: "multiple conditions - slow 5xx error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(15 * time.Millisecond)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			enabled:              false,
			slowQueriesThreshold: 10 * time.Millisecond,
			log5xxErrors:         true,
			expectedStatusCode:   http.StatusInternalServerError,
			shouldLog:            true,
			description:          "should log when multiple conditions are met",
		},
		{
			name: "implicit 200 status code",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("OK"))
			},
			enabled:              false,
			slowQueriesThreshold: 0,
			log5xxErrors:         true,
			expectedStatusCode:   http.StatusOK,
			shouldLog:            false,
			description:          "should handle implicit 200 status code correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockLog := &mockLogger{}
			enabled := atomic.Bool{}
			enabled.Store(tt.enabled)

			// Create the RequestLoggerHandler
			loggerHandler := RequestLoggerMiddleware(mockLog, &enabled, tt.slowQueriesThreshold, tt.log5xxErrors)(tt.handler)

			// Create a test HTTP request
			reqBody := "test body"
			req := httptest.NewRequest("POST", "http://example.com/foo", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Create a ResponseRecorder to record the response
			rr := httptest.NewRecorder()

			// Serve the HTTP request
			loggerHandler.ServeHTTP(rr, req)

			// Check the response status code
			assert.Equal(t, tt.expectedStatusCode, rr.Code, tt.description)

			// Check if logging occurred as expected
			loggedData := mockLog.GetLoggedData()
			if tt.shouldLog {
				assert.NotEmpty(t, loggedData, "expected request to be logged but no data was logged")
				assert.Contains(t, loggedData, "URI", "logged data should contain URI")
				assert.Contains(t, loggedData, "http://example.com/foo", "logged data should contain request URI")
				assert.Contains(t, loggedData, "Method", "logged data should contain Method")
				assert.Contains(t, loggedData, "POST", "logged data should contain request method")
				assert.Contains(t, loggedData, "Body", "logged data should contain Body")
				assert.Contains(t, loggedData, reqBody, "logged data should contain request body")

				// Check if timestamp is present
				foundTimestamp := false
				for i := 0; i < len(loggedData); i += 2 {
					if loggedData[i] == "Timestamp" {
						_, ok := loggedData[i+1].(int64)
						assert.True(t, ok, "timestamp should be an int64")
						foundTimestamp = true
						break
					}
				}
				assert.True(t, foundTimestamp, "timestamp should be logged")
			} else {
				assert.Empty(t, loggedData, "expected request not to be logged but data was logged: %v", loggedData)
			}
		})
	}
}
