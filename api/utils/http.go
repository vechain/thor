// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package utils

import (
	"encoding/json"
	"io"
	"net/http"
)

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
		if err != nil {
			if he, ok := err.(*httpError); ok {
				if he.cause != nil {
					http.Error(w, he.cause.Error(), he.status)
				} else {
					w.WriteHeader(he.status)
				}
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
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
	return json.NewEncoder(w).Encode(obj)
}

// M shortcut for type map[string]interface{}.
type M map[string]interface{}
