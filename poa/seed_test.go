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
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
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

	cache := make(map[thor.Bytes32][]byte)

	var b1ID thor.Bytes32
	binary.BigEndian.PutUint32(b1ID[:4], 1)
	var sig [65]byte
	rand.Read(sig[:])

	parent := b0
	for i := 1; i <= int(epochInterval*3); i++ {
		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Build().WithSignature(sig[:])

		if err := repo.AddBlock(b, nil, 0); err != nil {
			t.Fatal(err)
		}
		parent = b
	}
	if err := repo.SetBestBlockID(parent.Header().ID()); err != nil {
		t.Fatal(err)
	}

	b30ID, err := repo.NewBestChain().GetBlockID(epochInterval * 3)
	if err != nil {
		t.Fatal(err)
	}

	type fields struct {
		repo  *chain.Repository
		cache map[thor.Bytes32][]byte
	}
	type args struct {
		parentID thor.Bytes32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		{"early stage seeder should return nil slice", fields{repo, cache}, args{b1ID}, []byte(nil), false},
		{"seed block without beta should return nil slice", fields{repo, cache}, args{b30ID}, []byte(nil), false},
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
	parent, _ = repo.GetBlock(repo.BestBlockSummary().Header.ID())
	for i := 1; i <= int(epochInterval/2); i++ {
		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Build().WithSignature(sig[:])

		if err := repo.AddBlock(b, nil, 0); err != nil {
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

		parentBeta, err := sum.Header.Beta()
		if err != nil {
			t.Fatal(err)
		}
		if len(parentBeta) == 0 {
			parentBeta = thor.Bytes32{}.Bytes()
		}

		b := new(block.Builder).
			ParentID(parent.Header().ID()).
			Alpha(parentBeta).
			Build()

		sig, err := crypto.Sign(b.Header().SigningHash().Bytes(), priv)
		if err != nil {
			t.Fatal(err)
		}
		_, proof, err := vrf.Prove(priv, parentBeta)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := block.NewComplexSignature(sig, proof)
		if err != nil {
			t.Fatal(err)
		}

		b = b.WithSignature(cs)

		if err := repo.AddBlock(b, nil, 0); err != nil {
			t.Fatal(err)
		}
		parent = b
	}

	if err := repo.SetBestBlockID(parent.Header().ID()); err != nil {
		t.Fatal(err)
	}

	chain := repo.NewBestChain()
	b40, err := chain.GetBlockHeader(40)
	if err != nil {
		t.Fatal(err)
	}

	b40beta, err := b40.Beta()
	if err != nil {
		t.Fatal(err)
	}

	b51Seed := b40beta

	b51ID, err := chain.GetBlockID(51)
	if err != nil {
		t.Fatal(err)
	}
	b52ID, err := chain.GetBlockID(52)
	if err != nil {
		t.Fatal(err)
	}

	tests = []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		{"block 51 seed,should be block 40", fields{repo, cache}, args{b51ID}, b51Seed, false},
		{"block in 1 epoch should share seed", fields{repo, cache}, args{b52ID}, b51Seed, false},
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
