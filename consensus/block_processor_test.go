package consensus

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/vechain/thor/chain"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func Test_process(t *testing.T) {
	assert := assert.New(t)

	db, err := lvldb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stateC := state.NewCreator(db)

	tx := new(tx.Builder).
		GasPrice(big.NewInt(2)).
		Gas(math.MaxUint64).
		Clause(tx.NewClause(nil).WithData(common.Hex2Bytes("6060604052341561000f57600080fd5b61018d8061001e6000396000f30060606040526004361061006d576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806316e64048146100725780631f2a63c01461009b57806344650b74146100c457806367e06858146100e75780639650470c14610110575b600080fd5b341561007d57600080fd5b610085610133565b6040518082815260200191505060405180910390f35b34156100a657600080fd5b6100ae610139565b6040518082815260200191505060405180910390f35b34156100cf57600080fd5b6100e5600480803590602001909190505061013f565b005b34156100f257600080fd5b6100fa610149565b6040518082815260200191505060405180910390f35b341561011b57600080fd5b6101316004808035906020019091905050610157565b005b60005481565b60015481565b8060008190555050565b600060015460005401905090565b80600181905550505600a165627a7a723058201fb67fe068c521eaa014e518e0916c231e5e376eeabcad865a6c8a8619c34fca0029"))).
		Build()

	sender := genesis.Dev.Accounts()[0]

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), sender.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	receiptsRoot, err := thor.ParseHash("0xeaacdbfba6aa5324db4b9fb2d842e293ca6dbce1a12e58c08536bee69c0d2d43")
	if err != nil {
		t.Fatal(err)
	}

	genesisBlock, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}

	block := new(block.Builder).
		ParentID(genesisBlock.Header().ID()).
		Beneficiary(sender.Address).
		Timestamp(uint64(time.Now().Unix())).
		Transaction(tx.WithSignature(sig)).
		ReceiptsRoot(receiptsRoot).
		GasUsed(157592).
		Build()

	state, err := stateC.NewState(genesisBlock.Header().StateRoot())
	if err != nil {
		t.Fatal(err)
	}

	header := block.Header()
	totalReward, err := newBlockProcessor(
		runtime.New(
			state,
			header.Beneficiary(),
			header.Number(),
			header.Timestamp(),
			header.GasLimit(),
			nil), chain.New(db)).
		process(block, genesisBlock.Header())
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(totalReward, big.NewInt(94555))
}
