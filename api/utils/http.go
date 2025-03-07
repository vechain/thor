// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package utils

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/log"
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
	if boolStr == "" {
		return defaultVal, nil
	} else if boolStr == "false" {
		return false, nil
	} else if boolStr == "true" {
		return true, nil
	}
	return false, errors.New("should be boolean")
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
func ParseJSON(r io.Reader, v interface{}) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// WriteJSON response an object in JSON encoding.
func WriteJSON(w http.ResponseWriter, obj interface{}) error {
	w.Header().Set("Content-Type", JSONContentType)
	err := json.NewEncoder(w).Encode(obj)
	if err != nil {
		logger.Error("failed to write JSON response", "err", err)
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
type M map[string]interface{}
