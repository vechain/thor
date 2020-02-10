#!/bin/bash

HELP="launch_node -d [1-3] -b [bootnodeport] -v [0-9]"

while [ -n "$1" ]; do 
    case "$1" in
    -n) 
        ID=$2
        # echo "nodeid = $ID"
        shift
        ;;
    -v)
        VERB=$2
        shift
        ;;
    -b)
        BOOTNODEPORT=$2
        shift
        ;;
    -p)
        PORT=$2
        shift
        ;;
    *) 
        echo $HELP
        break
        ;;
    esac
    shift
done

# check node id (1-3)
if [ ! -n "$ID" ]; then
    echo "node id required: 1-3"
    echo $HELP
    return
fi

if [ ! -n "$PORT" ]; then
    PORT="11235"
fi

# echo "GOPATH=$GOPATH"
THORDIR="$GOPATH/src/github.com/vechain/thor"
THOR="$THORDIR/cmd/thor/thor"
DIR="$THORDIR/tmp/node$ID"
JSON="$THORDIR/tmp/test.json"

# Ubuntu
# BOOTNODE="enode://eb08ccf2668296b135e89d658f9a1a33408d8c7c9fe6c50b3501ff27265b2f8debc7f6d31e54e10d3faa47b9dcd919fa2e89cb05e0d8389e42909233baca89df@127.0.0.1:"
# Mac
BOOTNODE="enode://d047959848c3139b718a65ecce3eb4823accf05ccfa0be14d2ac552840582890c4c047c495152232d99f5c5ce670e82be154e280bc290d7dcebd7de67786400c@127.0.0.1:"

# PORT=11235
# while [ ! -z "$(sudo netstat -p udp | grep $PORT)" ]
# do
#     ((PORT=PORT+1))
# done

FILE=$(find $THORDIR/tmp/node1/ -iname peers.cache)
# echo "file1=$FILE"
if [ ! -z "$FILE" ]; then 
    rm $FILE
fi
FILE=$(find $THORDIR/tmp/node2/ -iname peers.cache)
# echo "file2=$FILE"
if [ ! -z "$FILE" ]; then 
    rm $FILE
fi
FILE=$(find $THORDIR/tmp/node3/ -iname peers.cache)
# echo "file3=$FILE"
if [ ! -z "$FILE" ]; then 
    rm $FILE
fi

CMD="$THOR --network $JSON --config-dir $DIR --data-dir $DIR --p2p-port $PORT --skip-logs"

echo "p2p port = $PORT"

if [ -n "$VERB" ]; then
    CMD="$CMD --verbosity $VERB"
fi

if [ -n "$BOOTNODEPORT" ]; then
    CMD="$CMD --bootnode $BOOTNODE$BOOTNODEPORT"
fi

if [ "$ID" == "2" ]; then
    CMD="$CMD --api-addr localhost:8670"
fi

if [ "$ID" == "3" ]; then
    CMD="$CMD --api-addr localhost:8671"
fi

$CMD