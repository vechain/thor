package api

import (
	"crypto/rand"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	// inject some invalid data to db
	data := db.NewStore("chain.data")
	var blkID thor.Bytes32
	rand.Read(blkID[:])
	data.Put(blkID[:], []byte("invalid data"))

	// get summary should fail since the block data is not rlp encoded
	_, err = repo.GetBlockSummary(blkID)
	assert.NotNil(t, err)

	//router := mux.NewRouter()
	//acc := accounts.New(repo, stater, math.MaxUint64, thor.NoFork, solo.NewBFTEngine(repo))
	//acc.Mount(router, "/accounts")
	//router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	//router.Use(metricsMiddleware)
	logdb, _ := logdb.NewMem()
	herp, _ := New(
		repo,
		stater,
		&txpool.TxPool{},
		logdb,
		solo.NewBFTEngine(repo),
		&solo.Communicator{},
		thor.ForkConfig{},
		"*",
		0,
		0,
		false,
		true,
		false,
		false,
		false,
		1000,
		true,
		true)
	ts := httptest.NewServer(herp)

	httpGet(t, ts.URL+"/accounts/"+thor.Address{}.String())

	// Test CORS headers
	//resp, _ := httpGet(t, ts.URL+"/accounts/0x")
	//assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	//assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	//
	//resp, _= httpGet(t, ts.URL+"/accounts/"+thor.Address{}.String())
	//assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	//assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
}
