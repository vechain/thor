package logger

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalStructLog(t *testing.T) {
	jsonInput := `{"test": "0x64"}`

	var log StructLog
	err := json.Unmarshal([]byte(jsonInput), &log)
	assert.NoError(t, err)
}
