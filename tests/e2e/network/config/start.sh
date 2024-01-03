#!/bin/sh

if [[ -n $MASTER_KEY_ADDRESS ]]; then
    echo "Starting node with master key address $MASTER_KEY_ADDRESS"

    cp /node/keys/$MASTER_KEY_ADDRESS/master.key /tmp
fi

BOOTNODE_IP=$(ping -c 1 thor-disco | awk -F'[()]' '/PING/{print $2}')

echo $BOOTNODE_IP

thor --config-dir=/tmp --network /node/config/genesis.json --api-addr="0.0.0.0:8669" --api-cors="*" --bootnode "enode://e32e5960781ce0b43d8c2952eeea4b95e286b1bb5f8c1f0c9f09983ba7141d2fdd7dfbec798aefb30dcd8c3b9b7cda8e9a94396a0192bfa54ab285c2cec515ab@$BOOTNODE_IP:55555"
