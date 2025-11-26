package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComplexStruct(t *testing.T) {
	method, ok := TestContract.ABI.MethodByName("getComplexStruct")
	assert.True(t, ok)
	t.Log(method.Name())
}
