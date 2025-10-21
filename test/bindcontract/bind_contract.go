// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bindcontract

const Code = `// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract TestContract {
    uint256 private value;
    address private owner;
    
    event ValueChanged(uint256 indexed oldValue, uint256 newValue);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    
    constructor() {
        owner = msg.sender;
        value = 42;
    }
    
    // Read operations
    function getValue() public view returns (uint256) {
        return value;
    }
    
    function getOwner() public view returns (address) {
        return owner;
    }
    
    // Write operations
    function setValue(uint256 newValue) public {
        uint256 oldValue = value;
        value = newValue;
        emit ValueChanged(oldValue, newValue);
    }
    
    function transferOwnership(address newOwner) public {
        require(msg.sender == owner, "not owner");
        address oldOwner = owner;
        owner = newOwner;
        emit OwnershipTransferred(oldOwner, newOwner);
    }
    
    // Payable function
    function deposit() public payable {
        // Just accept the payment
    }
}`

const ABI = `[
	{
		"inputs": [],
		"stateMutability": "nonpayable",
		"type": "constructor"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "previousOwner",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "newOwner",
				"type": "address"
			}
		],
		"name": "OwnershipTransferred",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "uint256",
				"name": "oldValue",
				"type": "uint256"
			},
			{
				"indexed": false,
				"internalType": "uint256",
				"name": "newValue",
				"type": "uint256"
			}
		],
		"name": "ValueChanged",
		"type": "event"
	},
	{
		"inputs": [],
		"name": "deposit",
		"outputs": [],
		"stateMutability": "payable",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "getOwner",
		"outputs": [
			{
				"internalType": "address",
				"name": "",
				"type": "address"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "getValue",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "newValue",
				"type": "uint256"
			}
		],
		"name": "setValue",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "newOwner",
				"type": "address"
			}
		],
		"name": "transferOwnership",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`

const HexBytecode = `0x608060405234801561001057600080fd5b5033600160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550602a6000819055506104cf806100696000396000f3fe60806040526004361061004a5760003560e01c8063209652551461004f578063552410771461007a578063893d20e8146100a3578063d0e30db0146100ce578063f2fde38b146100d8575b600080fd5b34801561005b57600080fd5b50610064610101565b60405161007191906102ee565b60405180910390f35b34801561008657600080fd5b506100a1600480360381019061009c919061033a565b61010a565b005b3480156100af57600080fd5b506100b8610153565b6040516100c591906103a8565b60405180910390f35b6100d661017d565b005b3480156100e457600080fd5b506100ff60048036038101906100fa91906103ef565b61017f565b005b60008054905090565b60008054905081600081905550807f2db947ef788961acc438340dbcb4e242f80d026b621b7c98ee306199503903828360405161014791906102ee565b60405180910390a25050565b6000600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b565b600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff161461020f576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161020690610479565b60405180910390fd5b6000600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905081600160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055508173ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff167f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e060405160405180910390a35050565b6000819050919050565b6102e8816102d5565b82525050565b600060208201905061030360008301846102df565b92915050565b600080fd5b610317816102d5565b811461032257600080fd5b50565b6000813590506103348161030e565b92915050565b6000602082840312156103505761034f610309565b5b600061035e84828501610325565b91505092915050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b600061039282610367565b9050919050565b6103a281610387565b82525050565b60006020820190506103bd6000830184610399565b92915050565b6103cc81610387565b81146103d757600080fd5b50565b6000813590506103e9816103c3565b92915050565b60006020828403121561040557610404610309565b5b6000610413848285016103da565b91505092915050565b600082825260208201905092915050565b7f6e6f74206f776e65720000000000000000000000000000000000000000000000600082015250565b600061046360098361041c565b915061046e8261042d565b602082019050919050565b6000602082019050818103600083015261049281610456565b905091905056fea2646970667358221220dde338433c727cae7c4dc53ef84b88f8ce492e974cab93a03ef202c78ff8adec64736f6c63430008110033`
