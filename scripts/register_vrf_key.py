#!/usr/bin/env python3
"""
Script for validators to register their public keys for VRF.
This script allows validators to update their public keys in the Staker contract.
"""

import argparse
import json
import sys
from eth_account import Account
from web3 import Web3
from eth_keys import keys
from eth_utils import to_checksum_address

# Configuration
STAKER_CONTRACT_ADDRESS = "0x0000000000000000000000000000456E65726779"  # Staker contract address

def load_abi():
    """Load Staker contract ABI"""
    # Simplified ABI for the functions we need
    return [
        {
            "inputs": [{"internalType": "address", "name": "master", "type": "address"}],
            "name": "getValidator",
            "outputs": [
                {"internalType": "address", "name": "", "type": "address"},
                {"internalType": "address", "name": "", "type": "address"},
                {"internalType": "bytes32", "name": "", "type": "bytes32"},
                {"internalType": "bool", "name": "", "type": "bool"},
                {"internalType": "bytes", "name": "", "type": "bytes"}
            ],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [
                {"internalType": "address", "name": "master", "type": "address"},
                {"internalType": "address", "name": "endorsor", "type": "address"},
                {"internalType": "bytes32", "name": "identity", "type": "bytes32"},
                {"internalType": "bytes", "name": "publicKey", "type": "bytes"}
            ],
            "name": "addValidator",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        }
    ]

def get_validator_info(w3, staker_contract, master_address):
    """Get validator information"""
    try:
        # Get validator info
        validator_info = staker_contract.functions.getValidator(master_address).call()
        
        if validator_info[0] == "0x0000000000000000000000000000000000000000":
            print(f"Error: No validator found for address {master_address}")
            return None
            
        return validator_info
    except Exception as e:
        print(f"Error getting validator information: {e}")
        return None

def register_public_key(w3, staker_contract, private_key, master_address, public_key_hex):
    """Register validator public key"""
    try:
        # Create account from private key
        account = Account.from_key(private_key)
        
        # Verify the account matches the master address
        if account.address.lower() != master_address.lower():
            print(f"Error: Private key does not match master address")
            print(f"Expected: {master_address}")
            print(f"Got: {account.address}")
            return False
        
        # Get current validator info
        validator_info = get_validator_info(w3, staker_contract, master_address)
        if not validator_info:
            return False
        
        print(f"Current validator info:")
        print(f"  Address: {validator_info[0]}")
        print(f"  Endorsor: {validator_info[1]}")
        print(f"  Identity: {validator_info[2]}")
        print(f"  Active: {validator_info[3]}")
        print(f"  Public Key: {validator_info[4].hex() if validator_info[4] else 'None'}")
        
        # Check if public key is already set
        if validator_info[4] and validator_info[4] != b'':
            print(f"Public key already set: {validator_info[4].hex()}")
            response = input("Do you want to update it? (y/N): ")
            if response.lower() != 'y':
                print("Operation cancelled")
                return False
        
        # Prepare transaction
        gas_estimate = staker_contract.functions.addValidator(
            master_address,
            validator_info[1],  # endorsor
            validator_info[2],  # identity
            public_key_hex.encode()  # public key
        ).estimate_gas({'from': account.address})
        
        print(f"Estimated gas: {gas_estimate}")
        
        # Build transaction
        tx = staker_contract.functions.addValidator(
            master_address,
            validator_info[1],  # endorsor
            validator_info[2],  # identity
            public_key_hex.encode()  # public key
        ).build_transaction({
            'from': account.address,
            'gas': int(gas_estimate * 1.2),  # Add 20% buffer
            'gasPrice': w3.eth.gas_price,
            'nonce': w3.eth.get_transaction_count(account.address)
        })
        
        # Sign and send transaction
        signed_tx = w3.eth.account.sign_transaction(tx, private_key)
        tx_hash = w3.eth.send_raw_transaction(signed_tx.rawTransaction)
        
        print(f"Transaction sent: {tx_hash.hex()}")
        print("Waiting for confirmation...")
        
        # Wait for transaction receipt
        receipt = w3.eth.wait_for_transaction_receipt(tx_hash)
        
        if receipt.status == 1:
            print("✅ Transaction successful!")
            
            # Get updated validator info
            updated_info = get_validator_info(w3, staker_contract, master_address)
            if updated_info:
                print(f"Updated public key: {updated_info[4].hex()}")
            
            return True
        else:
            print("❌ Transaction failed")
            return False
            
    except Exception as e:
        print(f"Error registering public key: {e}")
        return False

def main():
    parser = argparse.ArgumentParser(description='Register VRF public key for validator')
    parser.add_argument('--private-key', required=True, help='Validator private key')
    parser.add_argument('--rpc-url', required=True, help='RPC URL')
    parser.add_argument('--validator-id', required=True, help='Validator master address')
    parser.add_argument('--public-key', help='Public key (optional, will generate from private key if not provided)')
    
    args = parser.parse_args()
    
    try:
        # Connect to network
        w3 = Web3(Web3.HTTPProvider(args.rpc_url))
        
        if not w3.is_connected():
            print("Error: Cannot connect to network")
            sys.exit(1)
        
        # Load ABI and create contract
        abi = load_abi()
        staker_contract = w3.eth.contract(
            address=Web3.to_checksum_address(STAKER_CONTRACT_ADDRESS),
            abi=abi
        )
        
        # Get validator info
        validator_info = get_validator_info(w3, staker_contract, args.validator_id)
        if not validator_info:
            sys.exit(1)
        
        # Generate or use provided public key
        if args.public_key:
            public_key_hex = args.public_key
        else:
            # Generate from private key
            account = Account.from_key(args.private_key)
            public_key = keys.PrivateKey(bytes.fromhex(args.private_key[2:])).public_key
            public_key_hex = public_key.to_hex()
        
        print(f"Public key to register: {public_key_hex}")
        
        # Register public key
        success = register_public_key(w3, staker_contract, args.private_key, args.validator_id, public_key_hex)
        
        if success:
            print("The validator can now generate verifiable VRF proofs.")
        else:
            print("Failed to register public key")
            sys.exit(1)
            
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 