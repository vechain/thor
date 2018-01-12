package cry_test

import (
	"testing"

	"github.com/vechain/thor/thor"

	"github.com/vechain/thor/cry"
)

func TestSigning(t *testing.T) {

	s := cry.NewSigning(thor.Hash{})
	_ = s
}
