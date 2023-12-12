package block

import (
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var invalidData = []byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f, 0x70, 0x81, 0x92, 0xa3}

func getMockHeader() Header {
	// Create a mock valid header
	var header Header
	header.body.ParentID = thor.Bytes32{}
	header.body.Timestamp = 1234567890
	header.body.GasLimit = 1000000

	// Example address string
	addressStr := "0x8eaD0E8FEc8319Fd62C8508c71a90296EfD4F042"

	// Convert the string to a thor.Address
	address, err := thor.ParseAddress(addressStr)
	if err != nil {
		log.Fatalf("Error parsing address: %v", err)
	}

	header.body.Beneficiary = address
	header.body.GasUsed = 12345
	header.body.TotalScore = 1234
	header.body.TxsRootFeatures.Root = thor.Bytes32{}
	header.body.TxsRootFeatures.Features = 1234
	header.body.StateRoot = thor.Bytes32{}
	header.body.ReceiptsRoot = thor.Bytes32{}

	// Example signature (replace with real signature logic if needed)
	var sig [65]byte
	rand.Read(sig[:])
	header.body.Signature = sig[:]

	header.body.Extension.Alpha = []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	header.body.Extension.COM = true

	return header
}

// TestDecodeHeader tests the decoding of a block header from RLP data.
func TestRawDecodeHeader(t *testing.T) {

	header := getMockHeader()

	// Wrap the header in a slice
	headers := []*Header{&header}

	// Encode the slice of headers to a byte array
	encodedBytes, err := rlp.EncodeToBytes(headers)
	if err != nil {
		t.Fatalf("Failed to encode header list: %v", err)
	}

	// Output the encoded bytes for verification or further use
	t.Logf("Encoded Header: %x", encodedBytes)

	raw := Raw(encodedBytes)
	result, err := raw.DecodeHeader()
	if err != nil {
		t.Fatalf("DecodeHeader failed: %v", err)
	}

	t.Logf("Result 0x%x", result)
}

// TestDecodeHeaderWithError tests the DecodeHeader method with invalid data.
func TestRawDecodeHeaderWithError(t *testing.T) {
	raw := Raw(invalidData)
	_, err := raw.DecodeHeader()
	if err == nil {
		t.Fatal("DecodeHeader should have failed but it did not")
	}
}

// getMockBlock creates and returns a mock block for testing purposes.
func getMockBlock() *Block {
	tx1 := new(tx.Builder).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := new(tx.Builder).Clause(tx.NewClause(nil)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")

	now := uint64(time.Now().UnixNano())

	var (
		gasUsed     uint64       = 1000
		gasLimit    uint64       = 14000
		totalScore  uint64       = 101
		emptyRoot   thor.Bytes32 = thor.BytesToBytes32([]byte("0"))
		beneficiary thor.Address = thor.BytesToAddress([]byte("abc"))
	)

	block := new(Builder).
		GasUsed(gasUsed).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(now).
		ParentID(emptyRoot).
		Beneficiary(beneficiary).
		Build()

	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(block.Header().SigningHash().Bytes(), key)

	return block.WithSignature(sig)
}

// TestDecodeBody tests the decoding of a block body from RLP data.
func TestRawDecodeBody(t *testing.T) {

	block := getMockBlock()

	encodedBytes, err := rlp.EncodeToBytes(block)
	if err != nil {
		t.Fatal(err)
	}

	raw := Raw(encodedBytes)
	_, err = raw.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody failed: %v", err)
	}
}

// TestDecodeBodyWithError tests the DecodeBody method with invalid data.
func TestRawDecodeBodyWithError(t *testing.T) {
	raw := Raw(invalidData)
	_, err := raw.DecodeBody()
	if err == nil {
		t.Fatal("DecodeBody should have failed but it did not")
	}
}
