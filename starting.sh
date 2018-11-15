#!/bin/sh

/usr/local/bin/thor --network test --api-addr 0.0.0.0:0001 &&
echo initiating web3-gear &&
web3-gear --endpoint 0.0.0.0:0001 --host 0.0.0.0 --port 0002