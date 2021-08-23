// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"crypto/rand"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestSeeder_Generate(t *testing.T) {
	mockEpochInterval(10)
	db := muxdb.NewMem()
	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, err := chain.NewRepository(db, b0)
	if err != nil {
		t.Fatal(err)
	}

	cache := make(map[thor.Bytes32]thor.Bytes32)

	var b1ID thor.Bytes32
	binary.BigEndian.PutUint32(b1ID[:4], 1)
	var sig [65]byte
	rand.Read(sig[:])

	parent := b0
	for i := 1; i <= int(epochInterval*3); i++ {
		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Build().WithSignature(sig[:])

		if err := repo.AddBlock(b, nil); err != nil {
			t.Fatal(err)
		}
		parent = b
	}
	if err := repo.SetBestBlockID(parent.Header().ID()); err != nil {
		t.Fatal(err)
	}

	b2ID, err := repo.NewBestChain().GetBlockID(epochInterval * 3)
	if err != nil {
		t.Fatal(err)
	}

	type fields struct {
		repo  *chain.Repository
		cache map[thor.Bytes32]thor.Bytes32
	}
	type args struct {
		parentID thor.Bytes32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    thor.Bytes32
		wantErr bool
	}{
		{"early stage seeder should return empty bytes32", fields{repo, cache}, args{b1ID}, thor.Bytes32{}, false},
		{"seed block without beta should return empty bytes32", fields{repo, cache}, args{b2ID}, thor.Bytes32{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seeder := &Seeder{
				repo:  tt.fields.repo,
				cache: tt.fields.cache,
			}
			got, err := seeder.Generate(tt.args.parentID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Seeder.Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Seeder.Generate() = %v, want %v", got, tt.want)
			}
		})
	}

	// 31 - 35
	parent = repo.BestBlock()
	for i := 1; i <= int(epochInterval/2); i++ {
		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Build().WithSignature(sig[:])

		if err := repo.AddBlock(b, nil); err != nil {
			t.Fatal(err)
		}
		parent = b
	}

	// 36 -  55
	priv, _ := crypto.GenerateKey()
	for i := 1; i <= int(epochInterval*2); i++ {
		sum, err := repo.GetBlockSummary(parent.Header().ID())
		if err != nil {
			t.Fatal(err)
		}

		var beta []byte
		if len(sum.Beta()) > 0 {
			beta = sum.Beta()
		} else {
			beta = thor.Bytes32{}.Bytes()
		}

		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Alpha(beta).
			Build()

		sig, err := crypto.Sign(b.Header().SigningHash().Bytes(), priv)
		if err != nil {
			t.Fatal(err)
		}
		_, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(priv, beta)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := block.NewComplexSignature(proof, sig)
		if err != nil {
			t.Fatal(err)
		}

		b = b.WithSignature(cs)

		if err := repo.AddBlock(b, nil); err != nil {
			t.Fatal(err)
		}
		parent = b
	}

	// 56 - 65
	for i := 1; i <= int(epochInterval); i++ {
		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Build().WithSignature(sig[:])

		if err := repo.AddBlock(b, nil); err != nil {
			t.Fatal(err)
		}
		parent = b
	}
	if err := repo.SetBestBlockID(parent.Header().ID()); err != nil {
		t.Fatal(err)
	}

	var b51Seed thor.Bytes32
	hasher := thor.NewBlake2b()
	chain := repo.NewBestChain()
	for i := 40; i >= 36; i-- {
		id, err := chain.GetBlockID(uint32(i))
		if err != nil {
			t.Fatal(err)
		}

		sum, err := repo.GetBlockSummary(id)
		if err != nil {
			t.Fatal(err)
		}
		hasher.Write(sum.Beta())
	}
	hasher.Sum(b51Seed[:0])

	b51ID, err := chain.GetBlockID(51)
	if err != nil {
		t.Fatal(err)
	}
	b52ID, err := chain.GetBlockID(52)
	if err != nil {
		t.Fatal(err)
	}

	var b61Seed thor.Bytes32
	h := thor.NewBlake2b()
	for i := 50; i >= 41; i-- {
		id, err := chain.GetBlockID(uint32(i))
		if err != nil {
			t.Fatal(err)
		}

		sum, err := repo.GetBlockSummary(id)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(sum.Beta())
	}
	h.Sum(b61Seed[:0])

	b61ID, err := chain.GetBlockID(61)
	if err != nil {
		t.Fatal(err)
	}

	tests = []struct {
		name    string
		fields  fields
		args    args
		want    thor.Bytes32
		wantErr bool
	}{
		{"block 51 seed,should sum up from block 40 to 36", fields{repo, cache}, args{b51ID}, b51Seed, false},
		{"block in 1 epoch should share seed", fields{repo, cache}, args{b52ID}, b51Seed, false},
		{"block 61 seed,should sum up from block 50 to 41", fields{repo, cache}, args{b61ID}, b61Seed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seeder := &Seeder{
				repo:  tt.fields.repo,
				cache: tt.fields.cache,
			}
			got, err := seeder.Generate(tt.args.parentID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Seeder.Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Seeder.Generate() = %v, want %v", got, tt.want)
			}
		})
	}

}
