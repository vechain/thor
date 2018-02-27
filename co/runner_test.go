package co_test

import (
	"testing"

	"github.com/vechain/thor/co"
)

func TestRunner(t *testing.T) {
	var r co.Runner
	r.Go(func() {})
	r.Go(func() {})
	r.Wait()
}
