package txpool

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func SetupFile(t *testing.T, testFilePath string, dummyData string) {
	err := os.WriteFile(testFilePath, []byte(dummyData), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %s", err)
	}
}

func TestLoad(t *testing.T) {

	testFilePath := "test_blocklist.txt"
	// Create a test file with dummy data.
	// A list of unique valid Vechain addresses
	dummyData := "0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1"
	SetupFile(t, testFilePath, dummyData)

	var bl blocklist
	err := bl.Load(testFilePath)
	if err != nil {
		t.Errorf("Load failed: %s", err)
	}

	// Clean up: delete the test file.
	os.Remove(testFilePath)
}

func TestLoadWithError(t *testing.T) {

	testFilePath := "test_error_blocklist.txt"
	// Create a test file with invalid dummy data.
	// Expects valid Vechain addresses but invalid ones are given
	dummyData := "0x25Df024637d4\n0x25Df024637d4e56c1aE956"
	SetupFile(t, testFilePath, dummyData)

	var bl blocklist
	err := bl.Load(testFilePath)
	assert.Equal(t, err.Error(), "invalid length")
	assert.False(t, IsBadTx(err))
	assert.False(t, IsTxRejected(err))

	// Clean up: delete the test file.
	os.Remove(testFilePath)
}

func TestSave(t *testing.T) {

	testFilePath := "test_blocklist.txt"
	// Create a test file with dummy data.
	// A list of unique valid Vechain addresses
	dummyData := "0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1"
	SetupFile(t, testFilePath, dummyData)

	var bl blocklist
	err := bl.Load(testFilePath)
	if err != nil {
		t.Errorf("Load failed: %s", err)
	}

	// Clean up: delete the test file.
	os.Remove(testFilePath)

	// Test the Load function.
	err = bl.Save(testFilePath)
	if err != nil {
		t.Errorf("Load failed: %s", err)
	}

	fileContents, err := os.ReadFile(testFilePath)
	str := string(fileContents)
	assert.True(t, strings.Contains(str, "0x25df024637d4e56c1ae9563987bf3e92c9f534c0"))
	assert.True(t, strings.Contains(str, "0x25df024637d4e56c1ae9563987bf3e92c9f534c1"))

	// Clean up: delete the test file.
	os.Remove(testFilePath)
}

func TestLen(t *testing.T) {

	testFilePath := "test_blocklist.txt"
	// Create a test file with dummy data.
	// A list of unique valid Vechain addresses
	dummyData := "0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1"
	SetupFile(t, testFilePath, dummyData)

	var bl blocklist
	err := bl.Load(testFilePath)
	if err != nil {
		t.Errorf("Load failed: %s", err)
	}

	// Clean up: delete the test file.
	os.Remove(testFilePath)

	listLength := bl.Len()
	assert.Equal(t, listLength, 2)
}

func TestFetch(t *testing.T) {
	// Example data to be served by the mock server
	data := "0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1"

	expectedAddresses := []thor.Address{
		thor.MustParseAddress("0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0"),
		thor.MustParseAddress("0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1"),
	}

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// You can check headers, methods, etc. here
		if r.Header.Get("if-none-match") == "some-etag" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, data)
	}))
	defer server.Close()

	// Test scenarios
	tests := []struct {
		name    string
		etag    *string
		wantErr bool
	}{
		{"Successful Fetch", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bl blocklist
			bl.list = make(map[thor.Address]bool)

			// Set up ETAG if needed
			if tt.etag != nil {
				*tt.etag = "some-etag"
			}

			// Call the Fetch function
			err := bl.Fetch(context.Background(), server.URL, tt.etag)

			// Check for errors
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the blocklist contains the expected addresses
			for _, addr := range expectedAddresses {
				if _, exists := bl.list[addr]; !exists {
					t.Errorf("Fetch() missing address %s", addr)
				}
			}

		})
	}
}
