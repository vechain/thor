// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package loglevel

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

type TestCase struct {
	name             string
	method           string
	body             interface{}
	expectedStatus   int
	expectedLevel    string
	expectedErrorMsg string
}

func marshalBody(tt TestCase, t *testing.T) []byte {
	var reqBody []byte
	var err error
	if tt.body != nil {
		reqBody, err = json.Marshal(tt.body)
		if err != nil {
			t.Fatalf("could not marshal request body: %v", err)
		}
	}
	return reqBody
}

func TestLogLevelHandler(t *testing.T) {
	tests := []TestCase{
		{
			name:           "Valid POST input - set level to DEBUG",
			method:         "POST",
			body:           map[string]string{"level": "debug"},
			expectedStatus: http.StatusOK,
			expectedLevel:  "DEBUG",
		},
		{
			name:             "Invalid POST input - invalid level",
			method:           "POST",
			body:             map[string]string{"level": "invalid_body"},
			expectedStatus:   http.StatusBadRequest,
			expectedErrorMsg: "Invalid verbosity level",
		},
		{
			name:           "GET request - get current level INFO",
			method:         "GET",
			body:           nil,
			expectedStatus: http.StatusOK,
			expectedLevel:  "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logLevel slog.LevelVar
			logLevel.Set(slog.LevelInfo)

			reqBodyBytes := marshalBody(tt, t)

			req, err := http.NewRequest(tt.method, "/admin/loglevel", bytes.NewBuffer(reqBodyBytes))
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			router := mux.NewRouter()
			New(&logLevel).Mount(router, "/admin/loglevel")
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedLevel != "" {
				var response Response
				if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
					t.Fatalf("could not decode response: %v", err)
				}
				if response.CurrentLevel != tt.expectedLevel {
					t.Errorf("handler returned unexpected log level: got %v want %v", response.CurrentLevel, tt.expectedLevel)
				}
			} else {
				assert.Equal(t, tt.expectedErrorMsg, strings.Trim(rr.Body.String(), "\n"))
			}
		})
	}
}
