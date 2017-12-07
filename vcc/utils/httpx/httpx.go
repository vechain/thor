// Package httpx helper functions for http operations
package httpx

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
)

type errorWithStatus struct {
	err    error
	status int
}

func (e errorWithStatus) Error() string {
	return e.err.Error()
}

// Error create an error with http status code.
func Error(err error, status int) error {
	return errorWithStatus{
		err:    err,
		status: status,
	}
}

// HandlerFunc like http.HandlerFunc, bu it returns an error.
// If the returned error is errorWithStatus type, errorWithStatus.status will be responded,
// otherwise http.StatusInternalServerError responded.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// WrapHandlerFunc convert HandlerFunc to http.HandlerFunc
func WrapHandlerFunc(f HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := f(w, r)
		if err != nil {
			if se, ok := err.(errorWithStatus); ok {
				if se.err != nil {
					http.Error(w, se.err.Error(), se.status)
				} else {
					w.WriteHeader(se.status)
				}
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}
}

// content types
const (
	JSONContentType        = "application/json; charset=utf-8"
	OctetStreamContentType = "application/octet-stream"
)

// ResponseJSON reponse a object in JSON enconding
func ResponseJSON(w http.ResponseWriter, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", JSONContentType)
	w.Write(data)
	return nil
}

// HandleResponseError check response status code. If code is not 2xx, then error returned.
func HandleResponseError(resp *http.Response) error {
	if resp.StatusCode/100 == 2 {
		return nil
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return errors.Errorf("%s: %s", resp.Status, string(data))
}

// IsCausedByContextCanceled to check if the err
func IsCausedByContextCanceled(err error) bool {
	if err == context.Canceled {
		return true
	}
	if urlErr, ok := err.(*url.Error); ok && urlErr.Err == context.Canceled {
		return true
	}
	return false
}
