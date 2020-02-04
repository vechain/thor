ID=$1
HELP="launch_node [ 1 | 2 | 3 ]"

if [ "$ID" != "1" ] && [ "$ID" != "2" ] && [ "$ID" != "3" ]; then
    echo $HELP
    exit 1
fi

ROOT="$GOPATH/src/github.com/vechain/thor"
THOR="$ROOT/cmd/thor/thor"
DIR="$ROOT/tmp/node$ID"
JSON="$ROOT/tmp/test.json"

NODEID="enode://7680a68a552be39c833c4d00f1dd7e00b3f98ac34f5e8f7fabddebf271a6ff58d92e0607f55ebcdcfeceec2c3b658c63820af6a9f656af6a34123202532fc114@127.0.0.1:11235"

CMD="$THOR --network $JSON --config-dir $DIR --data-dir $DIR"

if [ "$ID" == "2" ]; then
    CMD="$CMD --api-addr localhost:8670 --p2p-port 11236 --bootnode $NODEID"
fi

if [ "$ID" == "3" ]; then
    CMD="$CMD --api-addr localhost:8671 --p2p-port 11237 --bootnode $NODEID"
fi

$CMD