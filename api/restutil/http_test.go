// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/tx"
)

func TestWrapHandlerFunc(t *testing.T) {
	handlerFunc := func(_ http.ResponseWriter, r *http.Request) error {
		return nil
	}
	wrapped := restutil.WrapHandlerFunc(handlerFunc)

	response := callWrappedFunc(&wrapped)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "", response.Body.String())
}

func TestWrapHandlerFuncWithGenericError(t *testing.T) {
	genericErrorMsg := "This is a generic error request"
	handlerFunc := func(_ http.ResponseWriter, r *http.Request) error {
		return errors.New(genericErrorMsg)
	}
	wrapped := restutil.WrapHandlerFunc(handlerFunc)

	response := callWrappedFunc(&wrapped)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, genericErrorMsg, strings.TrimSpace(response.Body.String()))
}

func TestWrapHandlerFuncWithBadRequestError(t *testing.T) {
	badMsg := "This is a bad request"
	handlerFunc := func(_ http.ResponseWriter, r *http.Request) error {
		return restutil.BadRequest(errors.New(badMsg))
	}
	wrapped := restutil.WrapHandlerFunc(handlerFunc)

	response := callWrappedFunc(&wrapped)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, badMsg, strings.TrimSpace(response.Body.String()))
}

func TestWrapHandlerFuncWithForbiddenError(t *testing.T) {
	forbiddenMsg := "This is a forbidden request"
	handlerFunc := func(w http.ResponseWriter, r *http.Request) error {
		return restutil.Forbidden(errors.New(forbiddenMsg))
	}
	wrapped := restutil.WrapHandlerFunc(handlerFunc)

	response := callWrappedFunc(&wrapped)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, forbiddenMsg, strings.TrimSpace(response.Body.String()))
}

func TestWrapHandlerFuncWithNilCauseError(t *testing.T) {
	errorStatus := http.StatusTeapot
	handlerFunc := func(w http.ResponseWriter, r *http.Request) error {
		return restutil.HTTPError(nil, errorStatus)
	}
	wrapped := restutil.WrapHandlerFunc(handlerFunc)

	response := callWrappedFunc(&wrapped)

	assert.Equal(t, errorStatus, response.Code)
	assert.Equal(t, "", response.Body.String())
}

func TestParseBlockRef(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  string
		wantRef  tx.BlockRef
		wantBadR bool
	}{
		{
			name:    "valid",
			input:   "0x0102030405060708",
			wantRef: tx.BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		},
		{
			name:     "malformed hex",
			input:    "not-hex",
			wantErr:  "blockRef: hex string without 0x prefix",
			wantBadR: true,
		},
		{
			name:     "invalid length",
			input:    "0x00",
			wantErr:  "blockRef: invalid length",
			wantBadR: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := restutil.ParseBlockRef(tt.input)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRef, ref)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
			// Malformed input must be classified as a client (400) error.
			handler := restutil.WrapHandlerFunc(func(_ http.ResponseWriter, _ *http.Request) error {
				return err
			})
			response := callWrappedFunc(&handler)
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func callWrappedFunc(wrapped *http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "http://example.com", nil)

	responseRec := httptest.NewRecorder()
	wrapped.ServeHTTP(responseRec, req)

	return responseRec
}

type mockReader struct {
	ID   int
	Body string
}

func TestParseJSON(t *testing.T) {
	var parsedRes mockReader
	body := mockReader{ID: 1, Body: "test"}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("GET", "http://example.com", bytes.NewReader(jsonBody))

	err := restutil.ParseJSON(req.Body, &parsedRes)

	assert.NoError(t, err)
	assert.Equal(t, body, parsedRes)
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	var body mockReader

	err := restutil.WriteJSON(rr, body)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, restutil.JSONContentType, rr.Header().Get("Content-Type"))

	respObj := mockReader{ID: 1, Body: "test"}
	err = json.NewDecoder(rr.Body).Decode(&respObj)

	assert.NoError(t, err)
	assert.Equal(t, body.ID, respObj.ID)
	assert.Equal(t, body.Body, respObj.Body)
}

// WriteJSONArray must be a drop-in replacement for WriteJSON on a slice: the wire
// format is part of the API contract and no client should be able to tell them apart.
func TestWriteJSONArrayMatchesWriteJSON(t *testing.T) {
	tests := []struct {
		name  string
		elems []*mockReader
	}{
		{name: "empty", elems: []*mockReader{}},
		{name: "single", elems: []*mockReader{{ID: 1, Body: "one"}}},
		{name: "many", elems: []*mockReader{{ID: 1, Body: "one"}, {ID: 2, Body: "two"}, {ID: 3, Body: "three"}}},
		{name: "escaped", elems: []*mockReader{{ID: 1, Body: `quote" slash\ unicode√`}}},
		{name: "nil element", elems: []*mockReader{nil, {ID: 2, Body: "two"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffered := httptest.NewRecorder()
			assert.NoError(t, restutil.WriteJSON(buffered, tt.elems))

			streamed := httptest.NewRecorder()
			assert.NoError(t, restutil.WriteJSONArray(streamed, len(tt.elems), func(i int) *mockReader {
				return tt.elems[i]
			}))

			assert.Equal(t, buffered.Body.String(), streamed.Body.String())
			assert.Equal(t, http.StatusOK, streamed.Code)
			assert.Equal(t, restutil.JSONContentType, streamed.Header().Get("Content-Type"))
		})
	}
}

// An empty result must serialize as [] rather than null, matching the pre-streaming
// behaviour where handlers passed a non-nil zero-length slice.
func TestWriteJSONArrayEmptyIsNotNull(t *testing.T) {
	rr := httptest.NewRecorder()

	assert.NoError(t, restutil.WriteJSONArray(rr, 0, func(int) *mockReader { return nil }))

	assert.Equal(t, "[]\n", rr.Body.String())
}

// countingWriter reports how many body bytes have reached the client so far.
type countingWriter struct {
	http.ResponseWriter
	written int
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.written += n
	return n, err
}

// The point of WriteJSONArray is that peak memory is one element, which holds only
// if elements reach the socket as they are produced. Assert that directly: with
// elements larger than the internal buffer, bytes must already be on the wire before
// the last element is even constructed. A reintroduced full-payload buffer would
// leave written at 0 until the very end.
func TestWriteJSONArrayStreamsIncrementally(t *testing.T) {
	const (
		elems   = 16
		bodyLen = 8 * 1024 // exceeds jsonArrayBufferSize once a few elements accumulate
	)

	cw := &countingWriter{ResponseWriter: httptest.NewRecorder()}
	writtenAt := make([]int, elems)

	assert.NoError(t, restutil.WriteJSONArray(cw, elems, func(i int) *mockReader {
		writtenAt[i] = cw.written
		return &mockReader{ID: i, Body: strings.Repeat("x", bodyLen)}
	}))

	assert.Zero(t, writtenAt[0], "nothing should be written before the first element is produced")
	assert.NotZero(t, writtenAt[elems-1], "earlier elements must already be on the wire")
	for i := 1; i < elems; i++ {
		assert.GreaterOrEqual(t, writtenAt[i], writtenAt[i-1], "written bytes must be monotonic")
	}
}

// failingWriter fails every write past the first.
type failingWriter struct {
	http.ResponseWriter
	writes int
}

func (f *failingWriter) Write(b []byte) (int, error) {
	f.writes++
	if f.writes > 1 {
		return 0, errors.New("connection reset")
	}
	return f.ResponseWriter.Write(b)
}

// Once the 200 is committed a failure cannot be reported as an HTTP error, so the
// array must be left unterminated: a truncated body is an unambiguous parse failure
// for the client, whereas a well-formed shorter array would look like a valid result.
func TestWriteJSONArrayWriteErrorLeavesArrayUnterminated(t *testing.T) {
	rec := httptest.NewRecorder()
	fw := &failingWriter{ResponseWriter: rec}

	// Elements large enough to force multiple writes through the internal buffer.
	err := restutil.WriteJSONArray(fw, 8, func(i int) *mockReader {
		return &mockReader{ID: i, Body: strings.Repeat("y", 8*1024)}
	})

	// Nil, like WriteJSON: the handler above cannot act on a broken response.
	assert.NoError(t, err)
	assert.NotContains(t, rec.Body.String(), "]", "closing bracket must not reach the client")

	var out []*mockReader
	assert.Error(t, json.Unmarshal(rec.Body.Bytes(), &out), "truncated body must not parse")
}

// unserialisable's channel field cannot be marshalled by encoding/json.
type unserialisable struct {
	Ch chan int `json:"ch"`
}

// Element 0 is marshalled before the ResponseWriter is touched, so a failure there
// must surface as an error rather than a committed, truncated 200 — and it must
// still be convertible into an HTTP error status by the caller.
func TestWriteJSONArrayFirstElementMarshalError(t *testing.T) {
	rr := httptest.NewRecorder()

	err := restutil.WriteJSONArray(rr, 1, func(int) *unserialisable { return &unserialisable{} })

	assert.Error(t, err)
	assert.Zero(t, rr.Body.Len(), "nothing should be written when element 0 fails to marshal")
	assert.Empty(t, rr.Header().Get("Content-Type"), "Content-Type must not be set before element 0 marshals successfully")

	handler := restutil.WrapHandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		return restutil.WriteJSONArray(w, 1, func(int) *unserialisable { return &unserialisable{} })
	})
	response := callWrappedFunc(&handler)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
}

// elem must be called exactly once per index, in order: element 0's marshalled
// result is reused for the write loop, not re-derived by calling elem(0) again.
func TestWriteJSONArrayCallsElemOncePerIndexInOrder(t *testing.T) {
	const n = 5
	calls := make([]int, n)
	var order []int

	rr := httptest.NewRecorder()
	err := restutil.WriteJSONArray(rr, n, func(i int) *mockReader {
		calls[i]++
		order = append(order, i)
		return &mockReader{ID: i}
	})

	assert.NoError(t, err)
	for i, c := range calls {
		assert.Equal(t, 1, c, "index %d should be called exactly once", i)
	}
	assert.Equal(t, []int{0, 1, 2, 3, 4}, order)
}
