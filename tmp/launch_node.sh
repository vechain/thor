#!/bin/bash

HELP="launch_node -d [1-3] -b [bootnodeport] -t [0-99] -v [0-9]"

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
    -t)
        MODE=$2
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
    exit 1
fi

if [ ! -n "$PORT" ]; then
    PORT="11235"
fi

if [ ! -n "$MODE" ]; then
    # echo "test mode required: 0-99"
    # echo $HELP
    # exit 1
    MODE="1"
fi

# echo "GOPATH=$GOPATH"
THORDIR="$GOPATH/src/github.com/vechain/thor"
THOR="$THORDIR/cmd/thor/thor"
DIR="$THORDIR/tmp/node$ID"
JSON="$THORDIR/tmp/test.json"

if [ "$(uname)" == "Darwin" ]; then 
    # Mac
    BOOTNODE="enode://d047959848c3139b718a65ecce3eb4823accf05ccfa0be14d2ac552840582890c4c047c495152232d99f5c5ce670e82be154e280bc290d7dcebd7de67786400c@127.0.0.1:"
else
    # Ubuntu
    BOOTNODE="enode://6508ef8879e64e63f748881c5337b22db092e051235cf5855c2070c78011531f4abd3b9114a2123d0684f6fad1763532571d502dc84ea1028f67a2ab412f576a@127.0.0.1:"
fi

# remove peer cache files
FILE=$(find $DIR -iname peers.cache)
if [ ! -z "$FILE" ]; then 
    rm $FILE
fi

CMD="$THOR --network $JSON --config-dir $DIR --data-dir $DIR --p2p-port $PORT --test-mode $MODE --skip-logs"

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