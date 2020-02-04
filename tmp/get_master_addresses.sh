#!/bin/bash

ROOT=${GOPATH}/src/github.com/vechain/thor

NODE1=${ROOT}/tmp/node1/
NODE2=${ROOT}/tmp/node2/
NODE3=${ROOT}/tmp/node3/


THORCMD=${ROOT}/cmd/thor/thor

$THORCMD master-key -config-dir $NODE1
$THORCMD master-key -config-dir $NODE2
$THORCMD master-key -config-dir $NODE3
