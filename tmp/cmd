# node1
thor master-key --config-dir <KEY_DIR1>
thor --network <JSON_FILE> --config-dir <KEY_DIR1> --data-dir <DATA_DIR1>

# node2
thor master-key --config-dir <KEY_DIR2>
thor --network <JSON_FILE> --config-dir <KEY_DIR2> --data-dir <DATA_DIR2> --api-addr localhost:8670 --p2p-port 11236 --bootnode enode://<ENODE_INFO_NODE1>@127.0.0.1:11235

# node3
thor master-key --config-dir <KEY_DIR3>
thor --network <JSON_FILE> --config-dir <KEY_DIR3> --data-dir <DATA_DIR3> --api-addr localhost:8671 --p2p-port 11237 --bootnode enode://<ENODE_INFO_NODE1>@127.0.0.1:11235