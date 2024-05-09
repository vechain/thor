// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockWriter implements filerotatewriter.FileRotateWriter interface
type MockWriter struct {
	Messages []string
}

func (mw *MockWriter) Start() error {
	return nil
}

func (mw *MockWriter) Write(p []byte) (int, error) {
	mw.Messages = append(mw.Messages, string(p))
	return len(p), nil
}

// TestRequestLoggerEnabled tests enabling of the RequestLogger
func TestRequestLoggerEnabled(t *testing.T) {
	mockWriter := &MockWriter{Messages: []string{}}
	logger := NewRequestLogger(true, mockWriter)
	assert.True(t, logger.Enabled(), "Logger should be enabled")
	logger.start()

	logger.Stop() // Ensuring logger stops correctly
	assert.Equal(t, 0, len(mockWriter.Messages), "There should be no messages logged")
}

// TestRequestLogging tests the logging of an HTTP request
func TestRequestLogging(t *testing.T) {
	mockWriter := &MockWriter{Messages: []string{}}
	logger := NewRequestLogger(true, mockWriter)
	handler := logger.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	logger.start()

	request := httptest.NewRequest("POST", "/test", bytes.NewBufferString("test body"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	time.Sleep(2 * time.Second) // logger is async, give some time to write to the mock struct

	assert.Equal(t, recorder.Result().Status, fmt.Sprintf("%d %s", http.StatusAccepted, http.StatusText(http.StatusAccepted)))
	assert.Greater(t, len(mockWriter.Messages), 0, "There should be at least one message logged")
	assert.Contains(t, mockWriter.Messages[0], "\"uri\":\"/test\"", "Log entry must contain the correct URI")
}

// Implement additional tests following the patterns above.
