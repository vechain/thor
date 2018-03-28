package utils

import (
	"encoding/json"
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
	JSONContentType        = "application/json; charset=utf-8"
	OctetStreamContentType = "application/octet-stream"
)

// WriteJSON reponse a object in JSON enconding.
func WriteJSON(w http.ResponseWriter, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return HTTPError(err, 500)
	}
	w.Header().Set("Content-Type", JSONContentType)
	w.Write(data)
	return nil
}

// M shortcut for type map[string]interface{}.
type M map[string]interface{}
