package bandwidth

import (
	"crypto/rand"
	"sync"
	"testing"

	"github.com/vechain/thor/v2/block"
)

func TestBandwidth(t *testing.T) {

	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	val := bandwidth.Value()

	if val != 0 {
		t.Errorf("Expected 0, got %d", val)
	}
}

func GetMockHeader(t *testing.T) *block.Header {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(block.Builder).Build().WithSignature(sig[:])
	h := block.Header()
	return h
}

func TestBandwithUpdate(t *testing.T) {

	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	header := block.NewMockHeader()
	bandwidth.Update(header, 1)
	val := bandwidth.Value()

	if val != 11234000000000 {
		t.Errorf("Expected 0, got %d", val)
	}
}

func TestBandwidthSuggestGasLimit(t *testing.T) {

	bandwidth := Bandwidth{
		value: 0,
		lock:  sync.Mutex{},
	}

	header := block.NewMockHeader()
	bandwidth.Update(header, 1)
	val := bandwidth.SuggestGasLimit()

	if val != 5617000000000 {
		t.Errorf("Expected 0, got %d", val)
	}
}
