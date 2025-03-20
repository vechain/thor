help:
	@egrep -h '\s#@\s' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?#@ "}; {printf "\033[36m  %-30s\033[0m %s\n", $$1, $$2}'

node1:| #@ Start node-1
	go run ../cmd/thor \
	   --config-dir ./node-1 \
	   --data-dir ./node-1 \
	   --network ./genesis.json \
       --api-cors="*" \
       --verbosity 4 \
       --beneficiary "0xbF89016e670595AAa225eEfa0a84B7FB17b8dAC8" \
       --allowed-peers "enode://13e2c6a93ad48b72b3f7f17151eff61583d76ca95485357035367d82901c052b94d7bc18b33b617017b4d16fde608a57c0a59a1e1c7bf3a58d6bbc15429fc793@0.0.0.0:11236,enode://9c4961d78701ba7b617788441842b028e3b4217e514dd59766cc88a96c3992d63fd8280e227b81b54ab957e96ef324fc920a852367df0b2da1297680b2184695@0.0.0.0:11237"

node2:| #@ Start node-2
	go run ../cmd/thor \
	   --config-dir ./node-2 \
	   --data-dir ./node-2 \
	   --network ./genesis.json \
	   --verbosity 4 \
	   --api-cors "*" --api-addr localhost:8668 --p2p-port 11236 \
       --allowed-peers "enode://53d97873a531504fd294d2496e093ad168afb73234e5ebdce5ba44342bb42c22ca1f7abb16473d0b0daa6f1ba8b4a15813a6de6782e98f598d9be6f64300df15@0.0.0.0:11235,enode://9c4961d78701ba7b617788441842b028e3b4217e514dd59766cc88a96c3992d63fd8280e227b81b54ab957e96ef324fc920a852367df0b2da1297680b2184695@0.0.0.0:11237"

node3:| #@ Start node-3
	go run ../cmd/thor \
	   --config-dir ./node-3 \
	   --data-dir ./node-3 \
	   --network ./genesis.json \
	  --api-cors "*" --api-addr localhost:8667 --p2p-port 11237 \
	  --allowed-peers "enode://53d97873a531504fd294d2496e093ad168afb73234e5ebdce5ba44342bb42c22ca1f7abb16473d0b0daa6f1ba8b4a15813a6de6782e98f598d9be6f64300df15@0.0.0.0:11235,enode://13e2c6a93ad48b72b3f7f17151eff61583d76ca95485357035367d82901c052b94d7bc18b33b617017b4d16fde608a57c0a59a1e1c7bf3a58d6bbc15429fc793@0.0.0.0:11236"

clean:| #@ Clean up
	rm -rf ./node-1/instance-*
	rm -rf ./node-2/instance-*
	rm -rf ./node-3/instance-*

kill:| #@ Stop all nodes
	pkill -f "go run ../cmd/thor"

.DEFAULT:
	@$(MAKE) help
