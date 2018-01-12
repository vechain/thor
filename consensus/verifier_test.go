package consensus_test

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/vechain/thor/cry"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestProcessBlock(t *testing.T) {
	sender := thor.Address(crypto.PubkeyToAddress(key.PublicKey))
	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Transaction(buildTransaction()).Build()
	//t.Log(consensus.ProcessBlock(block))
	_ = block
}

var key = func() *ecdsa.PrivateKey {
	key, _ := crypto.GenerateKey()
	return key
}()

func buildTransaction() *tx.Transaction {
	createClause := &tx.Clause{
		To:   nil,
		Data: common.Hex2Bytes("6060604052341561000f57600080fd5b61018d8061001e6000396000f30060606040526004361061006d576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806316e64048146100725780631f2a63c01461009b57806344650b74146100c457806367e06858146100e75780639650470c14610110575b600080fd5b341561007d57600080fd5b610085610133565b6040518082815260200191505060405180910390f35b34156100a657600080fd5b6100ae610139565b6040518082815260200191505060405180910390f35b34156100cf57600080fd5b6100e5600480803590602001909190505061013f565b005b34156100f257600080fd5b6100fa610149565b6040518082815260200191505060405180910390f35b341561011b57600080fd5b6101316004808035906020019091905050610157565b005b60005481565b60015481565b8060008190555050565b600060015460005401905090565b80600181905550505600a165627a7a723058201fb67fe068c521eaa014e518e0916c231e5e376eeabcad865a6c8a8619c34fca0029"),
	}

	tx := new(tx.Builder).GasPrice(big.NewInt(1)).Gas(5000000000000000000).Clause(createClause).Build()
	sig, _ := cry.NewSigning(thor.Hash{}).Sign(tx, crypto.FromECDSA(key))
	return tx.WithSignature(sig)
}
