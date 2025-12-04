// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/api/restutil"
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
