package kv_test

import (
	"testing"

	. "github.com/vechain/vecore/kv"
)

func TestStore(t *testing.T) {
	st, _ := NewMem(Options{})
	defer st.Close()
}
