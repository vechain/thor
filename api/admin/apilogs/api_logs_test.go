// Copyright (c) 2024 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package apilogs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/api"
)

type TestCase struct {
	name             string
	method           string
	expectedHTTP     int
	startValue       bool
	expectedEndValue bool
	requestBody      bool
}

func marshalBody(tt TestCase, t *testing.T) []byte {
	var reqBody []byte
	var err error
	if tt.method == "POST" {
		reqBody, err = json.Marshal(api.LogStatus{Enabled: tt.requestBody})
		if err != nil {
			t.Fatalf("could not marshal request body: %v", err)
		}
	}
	return reqBody
}

func TestLogLevelHandler(t *testing.T) {
	tests := []TestCase{
		{
			name:             "Valid POST input - set logs to enabled",
			method:           "POST",
			expectedHTTP:     http.StatusOK,
			startValue:       false,
			requestBody:      true,
			expectedEndValue: true,
		},
		{
			name:             "Valid POST input - set logs to disabled",
			method:           "POST",
			expectedHTTP:     http.StatusOK,
			startValue:       true,
			requestBody:      false,
			expectedEndValue: false,
		},
		{
			name:             "GET request - get current level INFO",
			method:           "GET",
			expectedHTTP:     http.StatusOK,
			startValue:       true,
			expectedEndValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLevel := atomic.Bool{}
			logLevel.Store(tt.startValue)

			reqBodyBytes := marshalBody(tt, t)

			req, err := http.NewRequest(tt.method, "/admin/apilogs", bytes.NewBuffer(reqBodyBytes))
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			router := mux.NewRouter()
			New(&logLevel).Mount(router, "/admin/apilogs")
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedHTTP, rr.Code)
			responseBody := api.LogStatus{}
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseBody))
			assert.Equal(t, tt.expectedEndValue, responseBody.Enabled)
		})
	}
}
