package kv_test

import (
	"testing"

	. "github.com/vechain/thor/kv"
)

func TestStore(t *testing.T) {
	st, _ := NewMem(Options{})
	defer st.Close()
}
