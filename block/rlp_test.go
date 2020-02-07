package block

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func encAndDec(t *testing.T, dat interface{}, decode interface{}) {
	buf := bytes.NewBuffer([]byte{})
	if err := rlp.Encode(buf, dat); err != nil {
		t.Fatal(err)
	}

	if err := rlp.DecodeBytes(buf.Bytes(), decode); err != nil {
		t.Fatal(err)
	}
}

func TestRLP(t *testing.T) {
	triggers := make(map[string]func())
	triggers["TestBlockSummaryRLP"] = func() {
		raw := RandBlockSummary()
		decode := new(Summary)
		encAndDec(t, raw, decode)
		assert.Equal(t, raw, decode)
	}
	triggers["TestEndorsementRLP"] = func() {
		raw := RandEndorsement(RandBlockSummary())
		decode := new(Endorsement)
		encAndDec(t, raw, decode)
		assert.Equal(t, raw, decode)
	}
	triggers["TestTxSetRLP"] = func() {
		raw := RandTxSet(1)
		decode := new(TxSet)
		encAndDec(t, raw, decode)
		assert.Equal(t, raw.ID(), decode.ID())
	}
	triggers["TestHeaderRLP"] = func() {
		raw := RandBlockHeader()
		decode := new(Header)
		encAndDec(t, raw, decode)
		assert.Equal(t, raw.ID(), decode.ID())
	}
	for _, trigger := range triggers {
		trigger()
	}
}
