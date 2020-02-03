package consensus

import (
	"crypto/rand"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
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
	rand.Read(msg)
	proof, err = sk.Prove(msg)
	if err != nil {
		t.Error(err)
	}

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg), math.MaxUint32)
	assert.Nil(t, err)
	assert.True(t, ok)
	assert.Equal(t, proof, pf)

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg), 0)
	assert.Nil(t, err)
	assert.False(t, ok)
	assert.Nil(t, pf)
}

func M(a ...interface{}) []interface{} {
	return a
}

func TestEpochNumber(t *testing.T) {
	tc, _ := NewTempChain(thor.NoFork)

	launchTime := tc.GenesisBlock.Header().Timestamp()

	tests := []struct {
		expected interface{}
		returned interface{}
		msg      string
	}{
		{
			[]interface{}{uint32(0)},
			M(tc.Con.EpochNumber(launchTime - 1)),
			"t < launch_time",
		},
		{
			[]interface{}{uint32(0)},
			M(tc.Con.EpochNumber(launchTime + 1)),
			"t = launch_time + 1",
		},
		{
			[]interface{}{uint32(1)},
			M(tc.Con.EpochNumber(launchTime + thor.BlockInterval)),
			"t = launch_time + block_interval",
		},
		{
			[]interface{}{uint32(1)},
			M(tc.Con.EpochNumber(launchTime + thor.BlockInterval*thor.EpochInterval)),
			"t = launch_time + block_interval * epoch_interval",
		},
		{
			[]interface{}{uint32(1)},
			M(tc.Con.EpochNumber(launchTime + thor.BlockInterval*thor.EpochInterval + 1)),
			"t = launch_time + block_interval * epoch_interval + 1",
		},
		{
			[]interface{}{uint32(2)},
			M(tc.Con.EpochNumber(launchTime + thor.BlockInterval*(thor.EpochInterval+1))),
			"t = launch_time + block_interval * (epoch_interval + 1)",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.returned, test.msg)
	}
}

func TestValidateBlockSummary(t *testing.T) {
	tc, _ := NewTempChain(thor.NoFork)
	tc.NewBlock(1, nil)

	cons := tc.Con
	sk := tc.Proposer.Ethsk
	header := tc.Original.Header()
	parentHeader := tc.Parent.Header()
	now := header.Timestamp() + 1

	triggers := make(map[string]func())
	triggers["triggerCleanCase"] = func() {
		bs := block.NewBlockSummary(
			header.ParentID(),
			header.TxsRoot(),
			header.Timestamp(),
			header.TotalScore(),
		)
		sig, _ := crypto.Sign(bs.SigningHash().Bytes(), sk)
		bs = bs.WithSignature(sig)
		actual := cons.ValidateBlockSummary(bs, parentHeader, now)
		assert.Nil(t, actual)
	}

	triggers["triggerInvalidParentID"] = func() {
		bs := block.NewBlockSummary(
			header.ParentID(),
			header.TxsRoot(),
			header.Timestamp(),
			header.TotalScore(),
		)
		sig, _ := crypto.Sign(bs.SigningHash().Bytes(), sk)
		bs = bs.WithSignature(sig)
		actual := cons.ValidateBlockSummary(bs, parentHeader, now)
		assert.Nil(t, actual)
	}

	triggers["triggerInvalidTotalScore"] = func() {
		bs := block.NewBlockSummary(
			header.ParentID(),
			header.TxsRoot(),
			header.Timestamp(),
			header.TotalScore()+1,
		)
		sig, _ := crypto.Sign(bs.SigningHash().Bytes(), sk)
		bs = bs.WithSignature(sig)
		actual := cons.ValidateBlockSummary(bs, parentHeader, now).Error()
		expected := newConsensusError(trLeader, strErrTotalScore,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{header.TotalScore(), bs.TotalScore()}, "").AddTraceInfo(trBlockSummary).Error()
		assert.Equal(t, expected, actual)
	}

	triggers["triggerInvalidTimestamp"] = func() {
		timestamps := []uint64{
			header.Timestamp() + 1,
			header.Timestamp() - 1,
			parentHeader.Timestamp(),
		}

		for _, timestamp := range timestamps {
			bs := block.NewBlockSummary(
				header.ParentID(),
				header.TxsRoot(),
				timestamp,
				header.TotalScore(),
			)
			sig, _ := crypto.Sign(bs.SigningHash().Bytes(), sk)
			bs = bs.WithSignature(sig)
			actual := cons.ValidateBlockSummary(bs, parentHeader, now).Error()
			expected := newConsensusError(trBlockSummary, strErrTimestamp,
				[]string{strDataTimestamp, strDataNowTime},
				[]interface{}{bs.Timestamp(), now}, "").Error()
			assert.Equal(t, expected, actual)
		}
	}

	triggers["triggerInvalidSignature"] = func() {
		bs := block.NewBlockSummary(
			header.ParentID(),
			header.TxsRoot(),
			header.Timestamp(),
			header.TotalScore(),
		)
		actual := cons.ValidateBlockSummary(bs, parentHeader, now).Error()
		expected := newConsensusError(trBlockSummary, strErrSignature, nil, nil, "invalid signature length").Error()
		assert.Equal(t, expected, actual)
	}

	triggers["triggerInvalidSigner"] = func() {
		bs := block.NewBlockSummary(
			header.ParentID(),
			header.TxsRoot(),
			header.Timestamp(),
			header.TotalScore(),
		)

		randKey, _ := crypto.GenerateKey()
		randSigner := thor.Address(crypto.PubkeyToAddress(randKey.PublicKey))

		sig, _ := crypto.Sign(bs.SigningHash().Bytes(), randKey)
		bs = bs.WithSignature(sig)
		actual := cons.ValidateBlockSummary(bs, parentHeader, now).Error()
		expected := newConsensusError(trLeader, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{randSigner}, "unauthorized block proposer").AddTraceInfo(trBlockSummary).Error()
		assert.Equal(t, expected, actual)
	}

	for _, trigger := range triggers {
		trigger()
	}
}

// func getValidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
// 	maxIter := 1000
// 	for i := 0; i < maxIter; i++ {
// 		pk, sk := vrf.GenKeyPair()
// 		proof, _ := sk.Prove(seed.Bytes())
// 		if isCommitteeByProof(proof) {
// 			return proof, pk
// 		}
// 	}
// 	return nil, nil
// }

// func getInvalidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
// 	maxIter := 1000
// 	for i := 0; i < maxIter; i++ {
// 		pk, sk := vrf.GenKeyPair()
// 		proof, _ := sk.Prove(seed.Bytes())
// 		if !isCommitteeByProof(proof) {
// 			return proof, pk
// 		}
// 	}
// 	return nil, nil
// }

func TestValidateEndorsement(t *testing.T) {
	tc, _ := NewTempChain(thor.NoFork)
	tc.NewBlock(1, nil)

	cons := tc.Con
	header := tc.Original.Header()
	parentHeader := tc.Parent.Header()
	ethsk := tc.Proposer.Ethsk
	vrfsk := tc.Proposer.Vrfsk

	// Create a valid block summary at round 1
	bs := block.NewBlockSummary(
		header.ParentID(),
		header.TxsRoot(),
		header.Timestamp(),
		header.TotalScore())
	sig, _ := crypto.Sign(bs.SigningHash().Bytes(), ethsk)
	bs = bs.WithSignature(sig)

	// compute vrf seed
	beacon := compBeacon(parentHeader)
	seed := seed(beacon, 1)

	triggers := make(map[string]func())
	triggers["triggerErrNotMasterNode"] = func() {
		proof, _ := vrfsk.Prove(seed.Bytes())
		ed := block.NewEndorsement(bs, proof)

		// random private key
		randKey, _ := crypto.GenerateKey()

		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), randKey)
		ed = ed.WithSignature(sig)
		signer, _ := ed.Signer()
		actual := cons.ValidateEndorsement(ed, parentHeader, bs.Timestamp()).Error()
		expected := newConsensusError(
			trEndorsement, strErrNotCandidate,
			[]string{strDataAddr}, []interface{}{signer}, "").Error()
		assert.Equal(t, expected, actual)
	}

	triggers["triggerErrInvalidProof"] = func() {
		// random vrf proof
		randProof := &vrf.Proof{}
		rand.Read(randProof[:])

		ed := block.NewEndorsement(bs, randProof)
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), ethsk)
		ed = ed.WithSignature(sig)
		actual := cons.ValidateEndorsement(ed, parentHeader, bs.Timestamp()).Error()
		expected := newConsensusError(
			trEndorsement, strErrProof, nil, nil, "").Error()
		assert.Equal(t, expected, actual)
	}

	triggers["triggerErrNotCommittee"] = func() {
		proof, _ := vrfsk.Prove(seed.Bytes())
		ed := block.NewEndorsement(bs, proof)
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), ethsk)
		ed = ed.WithSignature(sig)
		err := cons.ValidateEndorsement(ed, parentHeader, bs.Timestamp())
		if ok := IsCommitteeByProof(proof); !ok {
			actual := err.Error()
			expected := newConsensusError(trEndorsement, strErrNotCommittee, nil, nil, "").Error()
			assert.Equal(t, expected, actual)
		} else {
			assert.Nil(t, err)
		}
	}

	for _, trigger := range triggers {
		trigger()
	}
}

// func BenchmarkTestEthSig(b *testing.B) {
// 	sk, _ := crypto.GenerateKey()

// 	msg := make([]byte, 32)

// 	for i := 0; i < b.N; i++ {
// 		rand.Read(msg)
// 		crypto.Sign(msg, sk)
// 	}
// }

// func BenchmarkBeacon(b *testing.B) {
// 	cons, err := simpleConsensus()
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	for i := 0; i < b.N; i++ {
// 		cons.beacon(uint32(i + 1))
// 	}
// }
