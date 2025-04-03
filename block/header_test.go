// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"math/big"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestHeader_BetterThan(t *testing.T) {
	type fields struct {
		body  headerBody
		cache struct {
			signingHash atomic.Value
			id          atomic.Value
			pubkey      atomic.Value
			beta        atomic.Value
		}
	}
	type args struct {
		other *Header
	}

	var (
		largerID  fields
		smallerID fields
	)
	largerID.cache.id.Store(thor.Bytes32{1})
	smallerID.cache.id.Store(thor.Bytes32{0})

	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"higher score", fields{body: headerBody{TotalScore: 10}}, args{other: &Header{body: headerBody{TotalScore: 9}}}, true},
		{"lower score", fields{body: headerBody{TotalScore: 9}}, args{other: &Header{body: headerBody{TotalScore: 10}}}, false},
		{"equal score, larger id", largerID, args{&Header{smallerID.body, smallerID.cache}}, false},
		{"equal score, smaller id", smallerID, args{&Header{largerID.body, largerID.cache}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				body:  tt.fields.body,
				cache: tt.fields.cache,
			}
			if got := h.BetterThan(tt.args.other); got != tt.want {
				t.Errorf("Header.BetterThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderEncoding(t *testing.T) {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(Builder).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var hh Header
	err = rlp.DecodeBytes(bytes, &hh)
	if err != nil {
		t.Fatal(err)
	}

	bytes = append(bytes, []byte("just trailing")...)
	var hhh Header
	err = rlp.DecodeBytes(bytes, &hhh)
	assert.EqualError(t, err, "rlp: input contains more than one value")

	var proof [81]byte
	var alpha [32]byte
	rand.Read(proof[:])
	rand.Read(alpha[:])

	cplx, err := NewComplexSignature(sig[:], proof[:])
	if err != nil {
		t.Fatal(err)
	}

	b1 := new(Builder).Alpha(alpha[:]).Build().WithSignature(cplx[:])
	bs1, err := rlp.EncodeToBytes(b1.Header())
	if err != nil {
		t.Fatal(err)
	}

	var h1 Header
	err = rlp.DecodeBytes(bs1, &h1)
	if err != nil {
		t.Fatal(err)
	}
}

// type extension struct{Alpha []byte}
func TestEncodingBadExtension(t *testing.T) {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(Builder).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var h1 Header
	err = rlp.DecodeBytes(bytes, &h1)
	if err != nil {
		t.Fatal(err)
	}

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	// backward compatibility, required to be trimmed
	assert.EqualValues(t, 10, count)

	var raws []rlp.RawValue
	_ = rlp.DecodeBytes(bytes, &raws)
	d, _ := rlp.EncodeToBytes(&struct {
		Alpha []byte
	}{
		[]byte{},
	})
	raws = append(raws, d)
	b, _ := rlp.EncodeToBytes(raws)

	var h2 Header
	err = rlp.DecodeBytes(b, &h2)

	assert.EqualError(t, err, "rlp: extension must be trimmed")
}

// type extension struct{Alpha []byte}
func TestEncodingExtension(t *testing.T) {
	var sig [ComplexSigSize]byte
	var alpha [32]byte
	rand.Read(sig[:])
	rand.Read(alpha[:])

	block := new(Builder).Alpha(alpha[:]).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var hh Header
	err = rlp.DecodeBytes(bytes, &hh)
	if err != nil {
		t.Fatal(err)
	}

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 11, count)
}

// decoding block that generated by the earlier version
func TestCodingCompatibility(t *testing.T) {
	raw := hexutil.MustDecode("0xf8e0a0000000000000000000000000000000000000000000000000000000000000000080809400000000000000000000000000000000000000008080a045b0cfc220ceec5b7c1c62c4d4193d38e4eba48e8815729ce75f9c0ab0e4c1c0a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000b841e95a07bda136baa1181f32fba25b8dec156dee373781fdc7d24acd5e60ebc104c04b397ee7a67953e2d10acc4835343cd949a73e7e58db1b92f682db62e793c412")

	var h0 Header
	err := rlp.DecodeBytes(raw, &h0)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := rlp.EncodeToBytes(&h0)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, raw, bytes)

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 10, count)
}

type v2 struct {
	Extension extension
}

// type extension struct{Alpha []byte; COM bool, BaseFee *big.Int}
func TestExtensionV2(t *testing.T) {
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "default value",
			test: func(t *testing.T) {
				bytes, err := rlp.EncodeToBytes(&v2{
					Extension: extension{},
				})
				assert.Nil(t, err)

				content, _, err := rlp.SplitList(bytes)
				assert.Nil(t, err)

				cnt, err := rlp.CountValues(content)
				assert.Nil(t, err)

				assert.Equal(t, 0, cnt)

				var dst v2
				assert.Nil(t, rlp.DecodeBytes(bytes, &dst))
			},
		},
		{
			name: "regular",
			test: func(t *testing.T) {
				bytes, err := rlp.EncodeToBytes(&v2{
					Extension: extension{
						Alpha: thor.Bytes32{}.Bytes(),
						COM:   true,
					},
				})
				assert.Nil(t, err)

				content, _, err := rlp.SplitList(bytes)
				assert.Nil(t, err)

				cnt, err := rlp.CountValues(content)
				assert.Nil(t, err)

				// Extension should be present
				assert.Equal(t, 1, cnt)

				var dst v2
				err = rlp.DecodeBytes(bytes, &dst)
				assert.Nil(t, err)

				assert.Equal(t, thor.Bytes32{}.Bytes(), dst.Extension.Alpha)
				assert.True(t, dst.Extension.COM)
			},
		},
		{
			name: "only alpha",
			test: func(t *testing.T) {
				type v2x struct {
					Extension struct {
						Alpha []byte
					}
				}

				bytes, err := rlp.EncodeToBytes(&v2x{
					Extension: struct{ Alpha []byte }{
						Alpha: []byte{},
					},
				})
				assert.Nil(t, err)

				// Extension should be present in the Encoding
				content, _, err := rlp.SplitList(bytes)
				assert.Nil(t, err)
				cnt, err := rlp.CountValues(content)
				assert.Nil(t, err)
				assert.Equal(t, 1, cnt)

				// Continue split extension struct
				content, _, err = rlp.SplitList(content)
				assert.Nil(t, err)
				cnt, err = rlp.CountValues(content)
				assert.Nil(t, err)

				// Only alpha present
				assert.Equal(t, 1, cnt)

				var dst v2
				err = rlp.DecodeBytes(bytes, &dst)
				assert.EqualError(t, err, "rlp: extension must be trimmed")

				assert.False(t, dst.Extension.COM)
			},
		},
		{
			name: "alpha, com and baseFee",
			test: func(t *testing.T) {
				baseFee := big.NewInt(123456)
				bytes, err := rlp.EncodeToBytes(&v2{
					Extension: extension{
						Alpha:   thor.Bytes32{}.Bytes(),
						COM:     true,
						BaseFee: baseFee,
					},
				})
				assert.Nil(t, err)

				content, _, err := rlp.SplitList(bytes)
				assert.Nil(t, err)
				content, _, err = rlp.SplitList(content)
				assert.Nil(t, err)
				cnt, err := rlp.CountValues(content)
				assert.Nil(t, err)
				// All fields should be present
				assert.Equal(t, 3, cnt)

				var dst v2
				err = rlp.DecodeBytes(bytes, &dst)
				assert.Nil(t, err)

				assert.Equal(t, thor.Bytes32{}.Bytes(), dst.Extension.Alpha)
				assert.True(t, dst.Extension.COM)
				assert.Equal(t, baseFee, dst.Extension.BaseFee)
			},
		},
		{
			name: "alpha, com is false and baseFee",
			test: func(t *testing.T) {
				baseFee := big.NewInt(123456)
				bytes, err := rlp.EncodeToBytes(&v2{
					Extension: extension{
						Alpha:   thor.Bytes32{}.Bytes(),
						COM:     false,
						BaseFee: baseFee,
					},
				})
				assert.Nil(t, err)

				content, _, err := rlp.SplitList(bytes)
				assert.Nil(t, err)
				content, _, err = rlp.SplitList(content)
				assert.Nil(t, err)
				cnt, err := rlp.CountValues(content)
				assert.Nil(t, err)
				// All fields should be present
				assert.Equal(t, 3, cnt)

				var dst v2
				err = rlp.DecodeBytes(bytes, &dst)
				assert.Nil(t, err)

				assert.Equal(t, thor.Bytes32{}.Bytes(), dst.Extension.Alpha)
				assert.False(t, dst.Extension.COM)
				assert.Equal(t, baseFee, dst.Extension.BaseFee)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t)
		})
	}
}

func FuzzHeaderEncoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, addrBytes, beneficiary []byte, maxFee, gasUsed, gasLimit, totalScore uint64) {
		h0 := randomBlock(addrBytes, beneficiary, maxFee, gasUsed, gasLimit, totalScore).Header()
		enc, err := rlp.EncodeToBytes(h0)
		if err != nil {
			t.Errorf("failed to encode header: %v", err)
		}
		var decodedHeader Header
		if err := rlp.DecodeBytes(enc, &decodedHeader); err != nil {
			t.Errorf("failed to decode header: %v", err)
		}
		if h0.String() != decodedHeader.String() {
			t.Errorf("Header expected to be the same but: %v", err)
		}
	})
}

func TestHeaderHash(t *testing.T) {
	for range 100 {
		var builder = new(Builder)

		num := datagen.RandUint32()
		parentID := datagen.RandomHash()
		binary.BigEndian.PutUint32(parentID[:], num)
		u64 := datagen.RandUint64()

		builder.
			ParentID(parentID).
			Timestamp(u64 - u64%10).
			TotalScore(u64 + 100).
			GasLimit(u64 + 1000).
			GasUsed(u64 / 2).
			Beneficiary(datagen.RandAddress()).
			StateRoot(datagen.RandomHash()).
			ReceiptsRoot(datagen.RandomHash())

		var feat tx.Features
		if num%2 == 0 {
			feat.SetDelegated(true)
		}
		builder.TransactionFeatures(feat)

		if num%2 == 1 {
			builder.Alpha(datagen.RandomHash().Bytes())

			if num%3 == 0 {
				builder.COM()
			}

			if num%5 == 0 {
				builder.BaseFee(datagen.RandBigInt())
			}
		}
		h := builder.Build().Header()

		expectedFieldsLen := reflect.TypeOf(h.body).NumField() - 1
		if h.body.Extension.BaseFee == nil {
			expectedFieldsLen--
		}
		assert.Equal(t, expectedFieldsLen, len(h.signingFields()), "unexpected number of signing fields")

		expected := signingHash(h, t)
		assert.Equal(t, expected, h.SigningHash())
	}
}

// signingHash returns the signing hash the block header.
// this is a reflect based implementation used for cross checking.
func signingHash(h *Header, t *testing.T) thor.Bytes32 {
	types := reflect.TypeOf(h.body)
	values := reflect.ValueOf(h.body)

	fields := make([]any, 0)
	for i := range types.NumField() {
		// skip signature field
		if types.Field(i).Name != "Signature" {
			// pass extension field as a pointer
			if types.Field(i).Name == "Extension" {
				extension := values.Field(i).Interface().(extension)
				if extension.BaseFee != nil {
					fields = append(fields, &extension)
				}
			} else if types.Field(i).Name == "TxsRootFeatures" {
				rootFeat := values.Field(i).Interface().(txsRootFeatures)
				fields = append(fields, &rootFeat)
			} else {
				fields = append(fields, values.Field(i).Interface())
			}
		}
	}

	return thor.Blake2bFn(func(w io.Writer) {
		assert.Nil(t, rlp.Encode(w, fields))
	})
}
