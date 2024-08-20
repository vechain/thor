// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostLogLevelHandler_ValidInput(t *testing.T) {
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo)

	body := []byte(`{"level":"debug"}`)
	req, err := http.NewRequest("POST", "/admin/loglevel", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(HTTPHandler(&logLevel).ServeHTTP)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var response logLevelResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if response.CurrentLevel != "DEBUG" {
		t.Errorf("handler returned unexpected log level: got %v want %v", response.CurrentLevel, "DEBUG")
	}
}

func TestPostLogLevelHandler_InvalidInput(t *testing.T) {
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo)

	body := []byte(`{"level":"invalid_body"}`)
	req, err := http.NewRequest("POST", "/admin/loglevel", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(HTTPHandler(&logLevel).ServeHTTP)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expectedErrorMessage := "Invalid verbosity level"
	var response errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if response.ErrorMessage != expectedErrorMessage {
		t.Errorf("handler returned unexpected log level: got %v want %v", response.ErrorMessage, expectedErrorMessage)
	}
}

func TestGetLogLevelHandler(t *testing.T) {
	var logLevel slog.LevelVar

	req, err := http.NewRequest("GET", "/admin/loglevel", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(HTTPHandler(&logLevel).ServeHTTP)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var response logLevelResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if response.CurrentLevel != "INFO" {
		t.Errorf("handler returned unexpected log level: got %v want %v", response.CurrentLevel, "INFO")
	}
}
