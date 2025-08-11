package validation

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

var v = &Validation{
	Endorsor: datagen.RandAddress(),
	Period:             datagen.RandUint32(),
	CompleteIterations: datagen.RandUint32(),
	Status:             StatusActive,
	Online:             true,
	StartBlock:         datagen.RandUint32(),
	ExitBlock:          datagen.RandUint32(),
	LockedVET:          big.NewInt(1000),
	PendingUnlockVET:   big.NewInt(500),
	QueuedVET:          big.NewInt(2000),
	CooldownVET:        big.NewInt(3000),
	WithdrawableVET:    big.NewInt(4000),
	Weight:             big.NewInt(6000),
}

func TestValidation_Encode_Decode(t *testing.T) {
	slots := v.EncodeSlots()

	decoded := &Validation{}
	err := decoded.DecodeSlots(slots)
	require.NoError(t, err)

	require.Equal(t, v.Endorsor, decoded.Endorsor)
	require.Equal(t, v.Period, decoded.Period)
	require.Equal(t, v.CompleteIterations, decoded.CompleteIterations)
	require.Equal(t, v.Status, decoded.Status)
	require.Equal(t, v.Online, decoded.Online)
	require.Equal(t, v.StartBlock, decoded.StartBlock)
	require.Equal(t, v.ExitBlock, decoded.ExitBlock)
	require.Equal(t, v.LockedVET, decoded.LockedVET)
	require.Equal(t, v.PendingUnlockVET, decoded.PendingUnlockVET)
	require.Equal(t, v.QueuedVET, decoded.QueuedVET)
	require.Equal(t, v.CooldownVET, decoded.CooldownVET)
	require.Equal(t, v.WithdrawableVET, decoded.WithdrawableVET)
	require.Equal(t, v.Weight, decoded.Weight)
}


func TestValidation_WithState(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	param := params.New(thor.BytesToAddress([]byte("params")), st)
	param.Set(thor.KeyMaxBlockProposers, big.NewInt(101))

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(101)))
	sctx := solidity.NewContext(thor.BytesToAddress([]byte("staker")), st, nil)

	validations := NewRepository(sctx)

	address := datagen.RandAddress()

	assert.NoError(t, validations.SetValidation(address, v))

	decoded, err := validations.GetValidation(address)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	require.Equal(t, v.Endorsor, decoded.Endorsor)
	require.Equal(t, v.Period, decoded.Period)
	require.Equal(t, v.CompleteIterations, decoded.CompleteIterations)
	require.Equal(t, v.Status, decoded.Status)
	require.Equal(t, v.Online, decoded.Online)
	require.Equal(t, v.StartBlock, decoded.StartBlock)
	require.Equal(t, v.ExitBlock, decoded.ExitBlock)
	require.Equal(t, v.LockedVET, decoded.LockedVET)
	require.Equal(t, v.PendingUnlockVET, decoded.PendingUnlockVET)
	require.Equal(t, v.QueuedVET, decoded.QueuedVET)
	require.Equal(t, v.CooldownVET, decoded.CooldownVET)
	require.Equal(t, v.WithdrawableVET, decoded.WithdrawableVET)
	require.Equal(t, v.Weight, decoded.Weight)
}
