package consensus

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func TestThreshold(t *testing.T) {
	th := getCommitteeThreshold()
	// ratio = threhsold / (1 << 32 - 1) <= amp_factor * #committee / #node
	ratio := float64(th) / float64(math.MaxUint32)
	if ratio > float64(thor.CommitteeSize)/float64(thor.MaxBlockProposers)*float64(thor.CommitteeThresholdFactor) {
		t.Errorf("Invalid threshold")
	}
}

func TestIsCommitteeByPrivateKey(t *testing.T) {
	_, sk := vrf.GenKeyPair()

	// th := getCommitteeThreshold()

	var (
		msg       = make([]byte, 32)
		proof, pf *vrf.Proof
		err       error
		ok        bool
	)

	// Get a positive sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}

		if isCommitteeByProof(proof) {
			break
		}
	}

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg))
	if err != nil || !ok || pf == nil || bytes.Compare(pf[:], proof[:]) != 0 {
		t.Errorf("Testing positive sample failed")
	}

	// Get a negative sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}

		if !isCommitteeByProof(proof) {
			break
		}
	}

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg))
	if err != nil || ok || pf != nil {
		t.Errorf("Testing negative sample failed")
	}
}

func M(a ...interface{}) []interface{} {
	return a
}

func TestEpochNumber(t *testing.T) {
	_, cons, err := initConsensusTest()
	if err != nil {
		t.Fatal(err)
	}

	launchTime := cons.chain.GenesisBlock().Header().Timestamp()

	tests := []struct {
		expected interface{}
		returned interface{}
		msg      string
	}{
		{
			[]interface{}{uint32(0)},
			M(cons.EpochNumber(launchTime - 1)),
			"t < launch_time",
		},
		{
			[]interface{}{uint32(0)},
			M(cons.EpochNumber(launchTime + 1)),
			"t = launch_time + 1",
		},
		{
			[]interface{}{uint32(1)},
			M(cons.EpochNumber(launchTime + thor.BlockInterval)),
			"t = launch_time + block_interval",
		},
		{
			[]interface{}{uint32(1)},
			M(cons.EpochNumber(launchTime + thor.BlockInterval*thor.EpochInterval)),
			"t = launch_time + block_interval * epoch_interval",
		},
		{
			[]interface{}{uint32(1)},
			M(cons.EpochNumber(launchTime + thor.BlockInterval*thor.EpochInterval + 1)),
			"t = launch_time + block_interval * epoch_interval + 1",
		},
		{
			[]interface{}{uint32(2)},
			M(cons.EpochNumber(launchTime + thor.BlockInterval*(thor.EpochInterval+1))),
			"t = launch_time + block_interval * (epoch_interval + 1)",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.returned, test.msg)
	}
}

func TestValidateBlockSummary(t *testing.T) {
	sk := genesis.DevAccounts()[0].PrivateKey

	packer, cons, err := initConsensusTest()
	if err != nil {
		t.Fatal(err)
	}

	// Create a chain of round 1
	nRound := uint32(1)
	addEmptyBlocks(packer, cons.chain, sk, nRound, make(map[uint32]interface{}))

	best := cons.chain.BestBlock()
	round := nRound + 1
	now := cons.Timestamp(round) + 1

	type testObj struct {
		ParentID   thor.Bytes32
		TxsRoot    thor.Bytes32
		Timestamp  uint64
		TotalScore uint64
	}

	tests := []struct {
		input testObj
		ret   error
		msg   string
	}{
		{
			testObj{
				ParentID:   best.Header().ID(),
				TxsRoot:    thor.Bytes32{},
				Timestamp:  cons.Timestamp(round),
				TotalScore: 2},
			nil,
			"clean case",
		},
		{
			testObj{
				ParentID:   best.Header().ParentID(),
				TxsRoot:    thor.Bytes32{},
				Timestamp:  cons.Timestamp(round),
				TotalScore: 2},
			newConsensusError(trBlockSummary, strErrParentID, nil, nil, ""),
			"Invalid parent ID",
		},
		{
			testObj{
				ParentID:   best.Header().ID(),
				TxsRoot:    thor.Bytes32{},
				Timestamp:  cons.Timestamp(round) - 1,
				TotalScore: 2},
			newConsensusError(trBlockSummary, strErrTimestamp,
				[]string{strDataTimestamp, strDataNowTime},
				[]interface{}{cons.Timestamp(round) - 1, now}, ""),
			"Invalid timestamp",
		},
		{
			testObj{
				ParentID:   best.Header().ID(),
				TxsRoot:    thor.Bytes32{},
				Timestamp:  cons.Timestamp(round),
				TotalScore: 10},
			newConsensusError(trLeader, strErrTotalScore,
				[]string{strDataExpected, strDataCurr},
				[]interface{}{uint64(2), uint64(10)}, "").AddTraceInfo(trBlockSummary),
			"Invalid total score",
		},
	}

	for _, test := range tests {
		parentID := test.input.ParentID
		txsRoot := test.input.TxsRoot
		timestamp := test.input.Timestamp
		totalScore := test.input.TotalScore
		bs := block.NewBlockSummary(parentID, txsRoot, timestamp, totalScore)
		sig, _ := crypto.Sign(bs.SigningHash().Bytes(), sk)

		bs = bs.WithSignature(sig)

		actual := cons.ValidateBlockSummary(bs, best.Header(), now)
		expected := test.ret
		// assert.Equal(t, cons.ValidateBlockSummary(bs, best.Header(), test.input.Timestamp), test.ret, test.msg)
		assert.Equal(t, expected, actual)

		if actual != nil {
			fmt.Println(actual.Error())
		}
	}

	test := tests[0]
	parentID := test.input.ParentID
	txsRoot := test.input.TxsRoot
	timestamp := cons.Timestamp(round)
	totalScore := test.input.TotalScore
	bs := block.NewBlockSummary(parentID, txsRoot, timestamp, totalScore)
	actual := cons.ValidateBlockSummary(bs, best.Header(), test.input.Timestamp)
	expected := newConsensusError(trBlockSummary, strErrSignature, nil, nil, "invalid signature length")
	assert.Equal(t, expected, actual, "invalid signature")
}

func getValidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
	maxIter := 1000
	for i := 0; i < maxIter; i++ {
		pk, sk := vrf.GenKeyPair()
		proof, _ := sk.Prove(seed.Bytes())
		if isCommitteeByProof(proof) {
			return proof, pk
		}
	}
	return nil, nil
}

func getInvalidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
	maxIter := 1000
	for i := 0; i < maxIter; i++ {
		pk, sk := vrf.GenKeyPair()
		proof, _ := sk.Prove(seed.Bytes())
		if !isCommitteeByProof(proof) {
			return proof, pk
		}
	}
	return nil, nil
}

func TestValidateEndorsement(t *testing.T) {
	ethsk := genesis.DevAccounts()[0].PrivateKey
	_, vrfsk := vrf.GenKeyPairFromSeed(ethsk.D.Bytes())
	assert.Equal(t, vrfsk, genesis.DevAccounts()[0].VrfPrivateKey)

	_, cons, err := initConsensusTest()
	if err != nil {
		t.Fatal(err)
	}
	genHeader := cons.chain.GenesisBlock().Header()

	// Create a valid block summary at round 1
	bs := block.NewBlockSummary(genHeader.ID(), thor.Bytes32{}, genHeader.Timestamp()+thor.BlockInterval, 1)
	sig, _ := crypto.Sign(bs.SigningHash().Bytes(), ethsk)
	bs = bs.WithSignature(sig)

	// compute vrf seed
	beacon := getBeaconFromHeader(cons.chain.GenesisBlock().Header())
	seed := seed(beacon, 1)

	triggers := make(map[string]func())
	triggers["triggerErrNotMasterNode"] = func() {
		proof, _ := vrfsk.Prove(seed.Bytes())
		ed := block.NewEndorsement(bs, proof)
		sk, _ := crypto.GenerateKey()
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), sk)
		ed = ed.WithSignature(sig)
		signer, _ := ed.Signer()
		actual := cons.ValidateEndorsement(ed, genHeader, bs.Timestamp())
		expected := newConsensusError(
			trEndorsement, strErrNotCandidate,
			[]string{strDataAddr}, []interface{}{signer}, "")
		assert.Equal(t, actual, expected)
	}

	triggers["triggerErrInvalidProof"] = func() {
		proof := &vrf.Proof{}
		rand.Read(proof[:])
		ed := block.NewEndorsement(bs, proof)
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), ethsk)
		ed = ed.WithSignature(sig)
		actual := cons.ValidateEndorsement(ed, genHeader, bs.Timestamp())
		expected := newConsensusError(
			trEndorsement, strErrProof, nil, nil, "")
		assert.Equal(t, actual, expected)
	}

	triggers["triggerErrNotCommittee"] = func() {
		proof, _ := vrfsk.Prove(seed.Bytes())
		ed := block.NewEndorsement(bs, proof)
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), ethsk)
		ed = ed.WithSignature(sig)
		actual := cons.ValidateEndorsement(ed, genHeader, bs.Timestamp())
		if ok := IsCommitteeByProof(proof); !ok {
			expected := newConsensusError(trEndorsement, strErrNotCommittee, nil, nil, "")
			assert.Equal(t, actual, expected)
		} else {
			assert.Nil(t, actual)
		}
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func BenchmarkTestEthSig(b *testing.B) {
	sk, _ := crypto.GenerateKey()

	msg := make([]byte, 32)

	for i := 0; i < b.N; i++ {
		rand.Read(msg)
		crypto.Sign(msg, sk)
	}
}

func BenchmarkBeacon(b *testing.B) {
	_, cons, err := initConsensusTest()
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		cons.beacon(uint32(i + 1))
	}
}
