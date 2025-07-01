// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/thor"
)

func TestHandleXGenesisID(t *testing.T) {
	// Create a test genesis ID
	genesisID := thor.Bytes32{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	expectedID := genesisID.String()

	// Create middleware
	middleware := HandleXGenesisID(genesisID)

	// Create a simple handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap handler with middleware
	wrappedHandler := middleware(handler)

	tests := []struct {
		name           string
		headerValue    string
		queryValue     string
		expectedStatus int
		expectedBody   string
		checkHeader    bool
	}{
		{
			name:           "no header and no query parameter",
			headerValue:    "",
			queryValue:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
			checkHeader:    true,
		},
		{
			name:           "correct header value",
			headerValue:    expectedID,
			queryValue:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
			checkHeader:    true,
		},
		{
			name:           "correct query parameter value",
			headerValue:    "",
			queryValue:     expectedID,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
			checkHeader:    true,
		},
		{
			name:           "incorrect header value",
			headerValue:    "incorrect-id",
			queryValue:     "",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "genesis id mismatch\n",
			checkHeader:    true,
		},
		{
			name:           "incorrect query parameter value",
			headerValue:    "",
			queryValue:     "incorrect-id",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "genesis id mismatch\n",
			checkHeader:    true,
		},
		{
			name:           "header takes precedence over query parameter",
			headerValue:    expectedID,
			queryValue:     "incorrect-id",
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
			checkHeader:    true,
		},
		{
			name:           "incorrect header takes precedence over correct query parameter",
			headerValue:    "incorrect-id",
			queryValue:     expectedID,
			expectedStatus: http.StatusForbidden,
			expectedBody:   "genesis id mismatch\n",
			checkHeader:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("GET", "/test", nil)

			// Set header if provided
			if tt.headerValue != "" {
				req.Header.Set("x-genesis-id", tt.headerValue)
			}

			// Set query parameter if provided
			if tt.queryValue != "" {
				q := req.URL.Query()
				q.Set("x-genesis-id", tt.queryValue)
				req.URL.RawQuery = q.Encode()
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call the wrapped handler
			wrappedHandler.ServeHTTP(rr, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Check response body
			assert.Equal(t, tt.expectedBody, rr.Body.String())

			// Check that x-genesis-id header is always set in response
			if tt.checkHeader {
				assert.Equal(t, expectedID, rr.Header().Get("x-genesis-id"))
			}
		})
	}
}

func TestHandleXGenesisIDWithDifferentGenesisIDs(t *testing.T) {
	// Test with different genesis IDs
	genesisIDs := []thor.Bytes32{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20},
		{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8, 0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0, 0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8, 0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	for i, genesisID := range genesisIDs {
		t.Run(fmt.Sprintf("genesis_id_%d", i), func(t *testing.T) {
			expectedID := genesisID.String()
			middleware := HandleXGenesisID(genesisID)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Should always succeed with no header/query parameter
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, expectedID, rr.Header().Get("x-genesis-id"))
		})
	}
}

func TestHandleXGenesisIDWithBodyDiscard(t *testing.T) {
	// Test that request body is properly discarded when genesis ID mismatch occurs
	genesisID := thor.Bytes32{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	middleware := HandleXGenesisID(genesisID)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This should not be called when genesis ID mismatch occurs
		t.Error("Handler should not be called when genesis ID mismatch occurs")
	})

	wrappedHandler := middleware(handler)

	// Create request with incorrect genesis ID and a body
	req := httptest.NewRequest("POST", "/test", strings.NewReader("test body"))
	req.Header.Set("x-genesis-id", "incorrect-id")

	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	// Should return forbidden status
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t, "genesis id mismatch\n", rr.Body.String())
	assert.Equal(t, genesisID.String(), rr.Header().Get("x-genesis-id"))
}

func TestHandleXThorestVersion(t *testing.T) {
	// Create a simple handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap handler with middleware
	wrappedHandler := HandleXThorestVersion(handler)

	// Create request
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Call the wrapped handler
	wrappedHandler.ServeHTTP(rr, req)

	// Check that the response is successful
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())

	// Check that x-thorest-ver header is set in response
	version := rr.Header().Get("x-thorest-ver")
	assert.NotEmpty(t, version, "x-thorest-ver header should not be empty")

	// Verify that the version matches what doc.Version() returns
	expectedVersion := doc.Version()
	assert.Equal(t, expectedVersion, version, "x-thorest-ver header should match doc.Version()")
}

func TestHandleXThorestVersionWithDifferentHTTPMethods(t *testing.T) {
	// Test that the middleware works with different HTTP methods
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(r.Method))
	})

	wrappedHandler := HandleXThorestVersion(handler)

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Check that the response is successful
			assert.Equal(t, http.StatusOK, rr.Code)

			// Check that x-thorest-ver header is set
			version := rr.Header().Get("x-thorest-ver")
			assert.NotEmpty(t, version)
			assert.Equal(t, doc.Version(), version)
		})
	}
}

func TestHandleXThorestVersionWithErrorResponse(t *testing.T) {
	// Test that the middleware works even when the handler returns an error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	wrappedHandler := HandleXThorestVersion(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	// Check that the error response is preserved
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "internal server error")

	// Check that x-thorest-ver header is still set even for error responses
	version := rr.Header().Get("x-thorest-ver")
	assert.NotEmpty(t, version)
	assert.Equal(t, doc.Version(), version)
}

func TestHandleXThorestVersionHeaderConsistency(t *testing.T) {
	// Test that the header is consistently set across multiple requests
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := HandleXThorestVersion(handler)
	expectedVersion := doc.Version()

	// Make multiple requests to ensure consistency
	for i := range 5 {
		t.Run(fmt.Sprintf("request_%d", i), func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)

			version := rr.Header().Get("x-thorest-ver")
			assert.Equal(t, expectedVersion, version, "Version should be consistent across requests")
		})
	}
}

func TestHandleAPITimeout(t *testing.T) {
	// Test normal request with timeout
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := HandleAPITimeout(100 * time.Millisecond)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
}

func TestHandleAPITimeoutWithSlowHandler(t *testing.T) {
	// Test timeout with slow handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte("timeout"))
			return
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	})

	middleware := HandleAPITimeout(50 * time.Millisecond)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusRequestTimeout, rr.Code)
	assert.Equal(t, "timeout", rr.Body.String())
}

func TestHandleRequestBodyLimit(t *testing.T) {
	// Test normal request within limit
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	middleware := HandleRequestBodyLimit(100)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("POST", "/test", strings.NewReader("small body"))
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "small body", rr.Body.String())
}

func TestHandleRequestBodyLimitExceeded(t *testing.T) {
	// Test request body exceeds limit
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := HandleRequestBodyLimit(10)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest("POST", "/test", strings.NewReader("this body is too large"))
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	assert.Contains(t, rr.Body.String(), "http: request body too large")
}
