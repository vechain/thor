// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"encoding/hex"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

func M(a ...interface{}) []interface{} {
	return a
}
func TestContractSuicide(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()
	stater := state.NewStater(db)
	b0, _, _, err := g.Build(stater)
	assert.Nil(t, err)

	repo, _ := chain.NewRepository(db, b0)

	// contract:
	//
	// pragma solidity ^0.4.18;

	// contract TestSuicide {
	// 	function testSuicide() public {
	// 		selfdestruct(msg.sender);
	// 	}
	// }
	data, _ := hex.DecodeString("608060405260043610603f576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063085da1b3146044575b600080fd5b348015604f57600080fd5b5060566058565b005b3373ffffffffffffffffffffffffffffffffffffffff16ff00a165627a7a723058204cb70b653a3d1821e00e6ade869638e80fa99719931c9fa045cec2189d94086f0029")
	time := b0.Header().Timestamp()
	addr := thor.BytesToAddress([]byte("acc01"))
	state := stater.NewState(b0.Header().StateRoot(), 0, 0, 0)
	state.SetCode(addr, data)
	state.SetEnergy(addr, big.NewInt(100), time)
	state.SetBalance(addr, big.NewInt(200))

	abi, _ := abi.New([]byte(`[{
			"constant": false,
			"inputs": [],
			"name": "testSuicide",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`))
	suicide, _ := abi.MethodByName("testSuicide")
	methodData, err := suicide.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	origin := genesis.DevAccounts()[0].Address
	exec, _ := runtime.New(repo.NewChain(b0.Header().ID()), state, &xenv.BlockContext{Time: time}, thor.NoFork).
		PrepareClause(tx.NewClause(&addr).WithData(methodData), 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	expectedTransfer := &tx.Transfer{
		Sender:    addr,
		Recipient: origin,
		Amount:    big.NewInt(200),
	}
	assert.Equal(t, 1, len(out.Transfers))
	assert.Equal(t, expectedTransfer, out.Transfers[0])

	event, _ := builtin.Energy.ABI.EventByName("Transfer")
	expectedEvent := &tx.Event{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(addr.Bytes()), thor.BytesToBytes32(origin.Bytes())},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100},
	}
	assert.Equal(t, 1, len(out.Events))
	assert.Equal(t, expectedEvent, out.Events[0])

	assert.Equal(t, M(big.NewInt(0), nil), M(state.GetBalance(addr)))
	assert.Equal(t, M(big.NewInt(0), nil), M(state.GetEnergy(addr, time)))

	bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
	assert.Equal(t, M(new(big.Int).Add(bal, big.NewInt(200)), nil), M(state.GetBalance(origin)))
	assert.Equal(t, M(new(big.Int).Add(bal, big.NewInt(100)), nil), M(state.GetEnergy(origin, time)))
}

func TestChainID(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()

	stater := state.NewStater(db)
	b0, _, _, err := g.Build(stater)
	assert.Nil(t, err)

	repo, _ := chain.NewRepository(db, b0)

	// pragma solidity >=0.7.0 <0.9.0;
	// contract TestChainID {

	//     function chainID() public view returns (uint256) {
	//         return block.chainid;
	//     }
	// }
	data, _ := hex.DecodeString("6080604052348015600f57600080fd5b506004361060285760003560e01c8063adc879e914602d575b600080fd5b60336047565b604051603e9190605c565b60405180910390f35b600046905090565b6056816075565b82525050565b6000602082019050606f6000830184604f565b92915050565b600081905091905056fea264697066735822122060b67d944ffa8f0c5ee69f2f47decc3dc175ea2e4341a4de3705d72b868ce2b864736f6c63430008010033")
	addr := thor.BytesToAddress([]byte("acc01"))
	state := stater.NewState(b0.Header().StateRoot(), 0, 0, 0)
	state.SetCode(addr, data)

	abi, _ := abi.New([]byte(`[{
			"inputs": [],
			"name": "chainID",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`))
	chainIDMethod, _ := abi.MethodByName("chainID")
	methodData, err := chainIDMethod.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	exec, _ := runtime.New(repo.NewChain(b0.Header().ID()), state, &xenv.BlockContext{}, thor.ForkConfig{ETH_IST: 0}).
		PrepareClause(tx.NewClause(&addr).WithData(methodData), 0, math.MaxUint64, &xenv.TransactionContext{})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	assert.Equal(t, g.ID(), thor.BytesToBytes32(out.Data))
}

func TestSelfBalance(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()

	stater := state.NewStater(db)
	b0, _, _, err := g.Build(stater)
	assert.Nil(t, err)

	repo, _ := chain.NewRepository(db, b0)

	// pragma solidity >=0.7.0 <0.9.0;
	// contract TestSelfBalance {

	//     function selfBalance() public view returns (uint256) {
	//         return address(this).balance;
	//     }
	// }

	data, _ := hex.DecodeString("6080604052348015600f57600080fd5b506004361060285760003560e01c8063b0bed0ba14602d575b600080fd5b60336047565b604051603e9190605c565b60405180910390f35b600047905090565b6056816075565b82525050565b6000602082019050606f6000830184604f565b92915050565b600081905091905056fea2646970667358221220eeac1b7322c414db88987af09d3c8bdfde83bb378be9ac0e9ebe3fe34ecbcf2564736f6c63430008010033")
	addr := thor.BytesToAddress([]byte("acc01"))
	state := stater.NewState(b0.Header().StateRoot(), 0, 0, 0)
	state.SetCode(addr, data)
	state.SetBalance(addr, big.NewInt(100))

	abi, _ := abi.New([]byte(`[{
			"inputs": [],
			"name": "selfBalance",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`))
	selfBalanceMethod, _ := abi.MethodByName("selfBalance")
	methodData, err := selfBalanceMethod.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	exec, _ := runtime.New(repo.NewChain(b0.Header().ID()), state, &xenv.BlockContext{}, thor.ForkConfig{ETH_IST: 0}).
		PrepareClause(tx.NewClause(&addr).WithData(methodData), 0, math.MaxUint64, &xenv.TransactionContext{})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	assert.True(t, new(big.Int).SetBytes(out.Data).Cmp(big.NewInt(100)) == 0)
}

func TestBlake2(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()

	stater := state.NewStater(db)
	b0, _, _, err := g.Build(stater)
	assert.Nil(t, err)

	repo, _ := chain.NewRepository(db, b0)

	// pragma solidity >=0.7.0 <0.9.0;
	// contract TestBlake2 {
	// 	function F(uint32 rounds, bytes32[2] memory h, bytes32[4] memory m, bytes8[2] memory t, bool f) public view returns (bytes32[2] memory) {
	// 		bytes32[2] memory output;

	// 		bytes memory args = abi.encodePacked(rounds, h[0], h[1], m[0], m[1], m[2], m[3], t[0], t[1], f);

	// 		assembly {
	// 		  if iszero(staticcall(not(0), 0x09, add(args, 32), 0xd5, output, 0x40)) {
	// 			revert(0, 0)
	// 		  }
	// 		}

	// 		return output;
	// 	  }

	// 	  function callF() public view returns (bytes32[2] memory) {
	// 		uint32 rounds = 12;

	// 		bytes32[2] memory h;
	// 		h[0] = hex"48c9bdf267e6096a3ba7ca8485ae67bb2bf894fe72f36e3cf1361d5f3af54fa5";
	// 		h[1] = hex"d182e6ad7f520e511f6c3e2b8c68059b6bbd41fbabd9831f79217e1319cde05b";

	// 		bytes32[4] memory m;
	// 		m[0] = hex"6162630000000000000000000000000000000000000000000000000000000000";
	// 		m[1] = hex"0000000000000000000000000000000000000000000000000000000000000000";
	// 		m[2] = hex"0000000000000000000000000000000000000000000000000000000000000000";
	// 		m[3] = hex"0000000000000000000000000000000000000000000000000000000000000000";

	// 		bytes8[2] memory t;
	// 		t[0] = hex"03000000";
	// 		t[1] = hex"00000000";

	// 		bool f = true;

	// 		// Expected output:
	// 		// ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d1
	// 		// 7d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923
	// 		return F(rounds, h, m, t, f);
	// 	  }
	//   }
	data, _ := hex.DecodeString("608060405234801561001057600080fd5b50600436106100365760003560e01c806372de3cbd1461003b578063fc75ac471461006b575b600080fd5b61005560048036038101906100509190610894565b610089565b6040516100629190610a9b565b60405180910390f35b6100736102e5565b6040516100809190610a9b565b60405180910390f35b61009161063c565b61009961063c565b600087876000600281106100d6577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002015188600160028110610115577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002015188600060048110610154577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002015189600160048110610193577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201518a6002600481106101d2577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201518b600360048110610211577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201518b600060028110610250577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201518c60016002811061028f577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201518c6040516020016102ae9a999897969594939291906109e7565b604051602081830303815290604052905060408260d5602084016009600019fa6102d757600080fd5b819250505095945050505050565b6102ed61063c565b6000600c90506102fb61063c565b7f48c9bdf267e6096a3ba7ca8485ae67bb2bf894fe72f36e3cf1361d5f3af54fa581600060028110610356577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b6020020181815250507fd182e6ad7f520e511f6c3e2b8c68059b6bbd41fbabd9831f79217e1319cde05b816001600281106103ba577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b6020020181815250506103cb61065e565b7f616263000000000000000000000000000000000000000000000000000000000081600060048110610426577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b60200201818152505060008160016004811061046b577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b6020020181815250506000816002600481106104b0577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b6020020181815250506000816003600481106104f5577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002018181525050610506610680565b7f030000000000000000000000000000000000000000000000000000000000000081600060028110610561577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002019077ffffffffffffffffffffffffffffffffffffffffffffffff1916908177ffffffffffffffffffffffffffffffffffffffffffffffff1916815250506000816001600281106105de577f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b602002019077ffffffffffffffffffffffffffffffffffffffffffffffff1916908177ffffffffffffffffffffffffffffffffffffffffffffffff1916815250506000600190506106328585858585610089565b9550505050505090565b6040518060400160405280600290602082028036833780820191505090505090565b6040518060800160405280600490602082028036833780820191505090505090565b6040518060400160405280600290602082028036833780820191505090505090565b60006106b56106b084610adb565b610ab6565b905080828560208602820111156106cb57600080fd5b60005b858110156106fb57816106e18882610855565b8452602084019350602083019250506001810190506106ce565b5050509392505050565b600061071861071384610b01565b610ab6565b9050808285602086028201111561072e57600080fd5b60005b8581101561075e57816107448882610855565b845260208401935060208301925050600181019050610731565b5050509392505050565b600061077b61077684610b27565b610ab6565b9050808285602086028201111561079157600080fd5b60005b858110156107c157816107a7888261086a565b845260208401935060208301925050600181019050610794565b5050509392505050565b600082601f8301126107dc57600080fd5b60026107e98482856106a2565b91505092915050565b600082601f83011261080357600080fd5b6004610810848285610705565b91505092915050565b600082601f83011261082a57600080fd5b6002610837848285610768565b91505092915050565b60008135905061084f81610ca1565b92915050565b60008135905061086481610cb8565b92915050565b60008135905061087981610ccf565b92915050565b60008135905061088e81610ce6565b92915050565b600080600080600061014086880312156108ad57600080fd5b60006108bb8882890161087f565b95505060206108cc888289016107cb565b94505060606108dd888289016107f2565b93505060e06108ee88828901610819565b92505061012061090088828901610840565b9150509295509295909350565b60006109198383610993565b60208301905092915050565b61092e81610b57565b6109388184610b6f565b925061094382610b4d565b8060005b8381101561097457815161095b878261090d565b965061096683610b62565b925050600181019050610947565b505050505050565b61098d61098882610b7a565b610bfd565b82525050565b61099c81610b86565b82525050565b6109b36109ae82610b86565b610c0f565b82525050565b6109ca6109c582610b90565b610c19565b82525050565b6109e16109dc82610bbc565b610c23565b82525050565b60006109f3828d6109d0565b600482019150610a03828c6109a2565b602082019150610a13828b6109a2565b602082019150610a23828a6109a2565b602082019150610a3382896109a2565b602082019150610a4382886109a2565b602082019150610a5382876109a2565b602082019150610a6382866109b9565b600882019150610a7382856109b9565b600882019150610a83828461097c565b6001820191508190509b9a5050505050505050505050565b6000604082019050610ab06000830184610925565b92915050565b6000610ac0610ad1565b9050610acc8282610bcc565b919050565b6000604051905090565b600067ffffffffffffffff821115610af657610af5610c47565b5b602082029050919050565b600067ffffffffffffffff821115610b1c57610b1b610c47565b5b602082029050919050565b600067ffffffffffffffff821115610b4257610b41610c47565b5b602082029050919050565b6000819050919050565b600060029050919050565b6000602082019050919050565b600081905092915050565b60008115159050919050565b6000819050919050565b60007fffffffffffffffff00000000000000000000000000000000000000000000000082169050919050565b600063ffffffff82169050919050565b610bd582610c76565b810181811067ffffffffffffffff82111715610bf457610bf3610c47565b5b80604052505050565b6000610c0882610c35565b9050919050565b6000819050919050565b6000819050919050565b6000610c2e82610c87565b9050919050565b6000610c4082610c94565b9050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b6000601f19601f8301169050919050565b60008160e01b9050919050565b60008160f81b9050919050565b610caa81610b7a565b8114610cb557600080fd5b50565b610cc181610b86565b8114610ccc57600080fd5b50565b610cd881610b90565b8114610ce357600080fd5b50565b610cef81610bbc565b8114610cfa57600080fd5b5056fea2646970667358221220d54d4583b224c049d80665ae690afd0e7e998bf883c6b97472d292d1e2e5fa3e64736f6c63430008010033")
	addr := thor.BytesToAddress([]byte("acc01"))
	state := stater.NewState(b0.Header().StateRoot(), 0, 0, 0)
	state.SetCode(addr, data)

	abi, _ := abi.New([]byte(`[{
			"inputs": [
				{
					"internalType": "uint32",
					"name": "rounds",
					"type": "uint32"
				},
				{
					"internalType": "bytes32[2]",
					"name": "h",
					"type": "bytes32[2]"
				},
				{
					"internalType": "bytes32[4]",
					"name": "m",
					"type": "bytes32[4]"
				},
				{
					"internalType": "bytes8[2]",
					"name": "t",
					"type": "bytes8[2]"
				},
				{
					"internalType": "bool",
					"name": "f",
					"type": "bool"
				}
			],
			"name": "F",
			"outputs": [
				{
					"internalType": "bytes32[2]",
					"name": "",
					"type": "bytes32[2]"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "callF",
			"outputs": [
				{
					"internalType": "bytes32[2]",
					"name": "",
					"type": "bytes32[2]"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`))
	callFMethod, _ := abi.MethodByName("callF")
	methodData, err := callFMethod.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	exec, _ := runtime.New(repo.NewChain(b0.Header().ID()), state, &xenv.BlockContext{}, thor.ForkConfig{ETH_IST: 0}).
		PrepareClause(tx.NewClause(&addr).WithData(methodData), 0, math.MaxUint64, &xenv.TransactionContext{})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	var hashes [2][32]uint8
	callFMethod.DecodeOutput(out.Data, &hashes)

	assert.Equal(t, thor.MustParseBytes32("ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d1"), thor.Bytes32(hashes[0]))
	assert.Equal(t, thor.MustParseBytes32("7d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"), thor.Bytes32(hashes[1]))
}

func TestCall(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()
	b0, _, _, err := g.Build(state.NewStater(db))
	assert.Nil(t, err)

	repo, _ := chain.NewRepository(db, b0)

	state := state.New(db, b0.Header().StateRoot(), 0, 0, 0)

	rt := runtime.New(repo.NewChain(b0.Header().ID()), state, &xenv.BlockContext{}, thor.NoFork)

	method, _ := builtin.Params.ABI.MethodByName("executor")
	data, err := method.EncodeInput()
	assert.Nil(t, err)

	exec, _ := rt.PrepareClause(
		tx.NewClause(&builtin.Params.Address).WithData(data),
		0, math.MaxUint64, &xenv.TransactionContext{})
	out, _, err := exec()
	assert.Nil(t, err)
	assert.Nil(t, out.VMErr)

	var addr common.Address
	err = method.DecodeOutput(out.Data, &addr)
	assert.Nil(t, err)

	assert.Equal(t, thor.Address(addr), genesis.DevAccounts()[0].Address)

	// contract NeverStop {
	// 	constructor() public {
	// 		while(true) {
	// 		}
	// 	}
	// }
	data, _ = hex.DecodeString("6080604052348015600f57600080fd5b505b600115601b576011565b60358060286000396000f3006080604052600080fd00a165627a7a7230582026c386600e61384b3a93bf45760f3207b5cac072cec31c9cea1bc7099bda49b00029")
	exec, interrupt := rt.PrepareClause(tx.NewClause(nil).WithData(data), 0, math.MaxUint64, &xenv.TransactionContext{})

	go func() {
		interrupt()
	}()

	out, interrupted, err := exec()

	assert.NotNil(t, out)
	assert.True(t, interrupted)
	assert.Nil(t, err)
}

func TestExecuteTransaction(t *testing.T) {

	// kv, _ := lvldb.NewMem()

	// key, _ := crypto.GenerateKey()
	// addr1 := thor.Address(crypto.PubkeyToAddress(key.PublicKey))
	// addr2 := thor.BytesToAddress([]byte("acc2"))
	// balance1 := big.NewInt(1000 * 1000 * 1000)

	// b0, err := new(genesis.Builder).
	// 	Alloc(contracts.Energy.Address, &big.Int{}, contracts.Energy.RuntimeBytecodes()).
	// 	Alloc(addr1, balance1, nil).
	// 	Call(contracts.Energy.PackCharge(addr1, big.NewInt(1000000))).
	// 	Build(state.NewCreator(kv))

	// if err != nil {
	// 	t.Fatal(err)
	// }

	// tx := new(tx.Builder).
	// 	GasPrice(big.NewInt(1)).
	// 	Gas(1000000).
	// 	Clause(tx.NewClause(&addr2).WithValue(big.NewInt(10))).
	// 	Build()

	// sig, _ := crypto.Sign(tx.SigningHash().Bytes(), key)
	// tx = tx.WithSignature(sig)

	// state, _ := state.New(b0.Header().StateRoot(), kv)
	// rt := runtime.New(state,
	// 	thor.Address{}, 0, 0, 0, func(uint32) thor.Bytes32 { return thor.Bytes32{} })
	// receipt, _, err := rt.ExecuteTransaction(tx)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// _ = receipt
	// assert.Equal(t, state.GetBalance(addr1), new(big.Int).Sub(balance1, big.NewInt(10)))
}
