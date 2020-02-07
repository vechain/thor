ID=$1
# HELP="launch_node [ 1 | 2 | 3 ]"

# if [ "$ID" != "1" ] && [ "$ID" != "2" ] && [ "$ID" != "3" ]; then
#     echo $HELP
#     exit 1
# fi

ROOT="$GOPATH/src/github.com/vechain/thor"
THOR="$ROOT/cmd/thor/thor"
DIR="$ROOT/tmp/node$ID"
JSON="$ROOT/tmp/test.json"

NODEID="enode://d047959848c3139b718a65ecce3eb4823accf05ccfa0be14d2ac552840582890c4c047c495152232d99f5c5ce670e82be154e280bc290d7dcebd7de67786400c@127.0.0.1:"

PORT=11235
while [ ! -z "$(netstat -p udp | grep $PORT)" ]
do
    ((PORT=PORT+1))
done

rm $ROOT/tmp/node1/*/peers.Cache
rm $ROOT/tmp/node2/*/peers.Cache
rm $ROOT/tmp/node3/*/peers.Cache

CMD="$THOR --verbosity 9 --network $JSON --config-dir $DIR --data-dir $DIR --p2p-port $PORT"

echo "port = $PORT"

if [ "$ID" == "2" ]; then
    CMD="$CMD --api-addr localhost:8670 --bootnode $NODEID$2"
fi

if [ "$ID" == "3" ]; then
    CMD="$CMD --api-addr localhost:8671 --bootnode $NODEID$2"
fi

$CMD