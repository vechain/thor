package co_test

import (
	"testing"

	"github.com/vechain/thor/co"
)

func TestGoes(t *testing.T) {
	var g co.Goes
	g.Go(func() {})
	g.Go(func() {})
	g.Wait()

	<-g.Done()
}
