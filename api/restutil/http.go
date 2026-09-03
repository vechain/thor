// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var logger = log.WithContext("pkg", "http-utils")

type httpError struct {
	cause  error
	status int
}

func (e *httpError) Error() string {
	return e.cause.Error()
}

// HTTPError create an error with http status code.
func HTTPError(cause error, status int) error {
	return &httpError{
		cause:  cause,
		status: status,
	}
}

// BadRequest convenience method to create http bad request error.
func BadRequest(cause error) error {
	return &httpError{
		cause:  cause,
		status: http.StatusBadRequest,
	}
}

func StringToBoolean(boolStr string, defaultVal bool) (bool, error) {
	switch boolStr {
	case "":
		return defaultVal, nil
	case "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, errors.New("should be boolean")
}

func StringToAddress(addressString string) (*thor.Address, error) {
	var address *thor.Address
	if addressString != "" {
		fromParsed, err := thor.ParseAddress(addressString)
		if err != nil {
			return nil, err
		}
		address = &fromParsed
	}
	return address, nil
}

// Forbidden convenience method to create http forbidden error.
func Forbidden(cause error) error {
	return &httpError{
		cause:  cause,
		status: http.StatusForbidden,
	}
}

// HandlerFunc like http.HandlerFunc, bu it returns an error.
// If the returned error is httpError type, httpError.status will be responded,
// otherwise http.StatusInternalServerError responded.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// WrapHandlerFunc convert HandlerFunc to http.HandlerFunc.
func WrapHandlerFunc(f HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := f(w, r)
		if err == nil {
			return // No error, nothing to do
		}

		// Otherwise, proceed with normal HTTP error handling
		if he, ok := err.(*httpError); ok {
			if he.cause != nil {
				http.Error(w, he.cause.Error(), he.status)
			} else {
				w.WriteHeader(he.status)
			}
		} else {
			logger.Debug("all errors should be wrapped in httpError", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// content types
const (
	JSONContentType = "application/json; charset=utf-8"
)

// ParseJSON parse a JSON object using strict mode.
func ParseJSON(r io.Reader, v any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// ParseBlockRef parses a hex-encoded, 8-byte block reference from a request.
func ParseBlockRef(blockRef string) (tx.BlockRef, error) {
	var blkRef tx.BlockRef
	decoded, err := hexutil.Decode(blockRef)
	if err != nil {
		return blkRef, BadRequest(errors.WithMessage(err, "blockRef"))
	}
	if len(decoded) != len(blkRef) {
		return blkRef, BadRequest(errors.New("blockRef: invalid length"))
	}
	copy(blkRef[:], decoded)
	return blkRef, nil
}

// WriteJSON response an object in JSON encoding.
//
// json.Encoder.Encode marshals into an internal buffer and issues a single Write,
// so the whole payload is held in memory. Use WriteJSONArray for responses whose
// size is driven by a caller-supplied limit.
func WriteJSON(w http.ResponseWriter, obj any) error {
	w.Header().Set("Content-Type", JSONContentType)
	err := json.NewEncoder(w).Encode(obj)
	if err != nil {
		logger.Debug("failed to write JSON response", "err", err)
	}
	return nil
}

// jsonArrayBufferSize bounds the write buffer WriteJSONArray places in front of the
// ResponseWriter. It amortises the per-element write over the gzip/chunking layers
// without reintroducing a payload-sized buffer.
const jsonArrayBufferSize = 16 * 1024

// WriteJSONArray responds with a JSON array of n elements, marshalling one at a
// time so peak memory is the write buffer plus one element, not the whole array.
// Output is byte-identical to WriteJSON on the equivalent slice.
//
// The framing is hand-rolled: encoding/json has no incremental array API, and the
// stdlib's streaming token writer is gated behind GOEXPERIMENT=jsonv2.
//
// elem is called once per index, in order; its result is not retained after the
// element is written. The 200 is committed when the buffer first flushes, so any
// later failure can only abandon the array unterminated. Callers must finish all
// validation that can reject the request before calling this.
//
// Returns an error only when element 0 fails to marshal: nothing is committed yet,
// so the caller can still turn it into an error status.
func WriteJSONArray[T any](w http.ResponseWriter, n int, elem func(i int) T) error {
	// Marshal element 0 before touching the ResponseWriter, so a non-serialisable
	// T becomes an error status rather than a committed, truncated 200.
	var first []byte
	if n > 0 {
		b, err := json.Marshal(elem(0))
		if err != nil {
			return err
		}
		first = b
	}

	w.Header().Set("Content-Type", JSONContentType)

	// bufio latches the first write error, so once a write fails nothing further
	// reaches the client — including the closing bracket.
	bw := bufio.NewWriterSize(w, jsonArrayBufferSize)
	_ = bw.WriteByte('[')
	if n > 0 {
		if _, err := bw.Write(first); err != nil {
			return nil
		}
	}
	for i := 1; i < n; i++ {
		if err := bw.WriteByte(','); err != nil {
			return nil
		}
		b, err := json.Marshal(elem(i))
		if err != nil {
			// Unreachable once element 0 marshalled. The response is already committed
			// either way; the panic is for the stack trace naming the offending caller.
			panic(fmt.Errorf("restutil: element %d of the JSON array is not serialisable: %w", i, err))
		}
		if _, err := bw.Write(b); err != nil {
			// Client is gone; don't convert the rest.
			return nil
		}
	}
	_, _ = bw.WriteString("]\n")

	if err := bw.Flush(); err != nil {
		logger.Debug("failed to write JSON array response", "err", err)
	}
	return nil
}

// HandleGone is a handler for deprecated endpoints that returns HTTP 410 Gone.
func HandleGone(w http.ResponseWriter, _ *http.Request) error {
	w.WriteHeader(http.StatusGone)
	_, _ = w.Write([]byte("This endpoint is no longer supported."))
	return nil
}

// M shortcut for type map[string]interface{}.
type M map[string]any
