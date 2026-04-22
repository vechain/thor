ROOT       := $(shell pwd)
THOR       := $(ROOT)/bin/thor
THOR_SRC   := $(ROOT)/..
ETH_PORT   := 8545
API_PORT   := 8669
DAPP_PORT  := 3000

.PHONY: all build start install deploy increment dapp reset clean

all: build

## build — compile the thor binary from source
build:
	@echo ">>> Building thor..."
	@go build -C $(THOR_SRC) -o $(THOR) ./cmd/thor
	@echo ">>> Built: $(THOR)"

## start — build and run thor in solo mode (blocks every 10s, persists in ./data)
start: build
	@echo ">>> Starting Thor solo node..."
	@echo "    ETH RPC : http://localhost:$(ETH_PORT)"
	@echo "    API     : http://localhost:$(API_PORT)"
	$(THOR) solo \
		--eth-rpc-addr localhost:$(ETH_PORT) \
		--api-addr     localhost:$(API_PORT) \
		--data-dir     $(ROOT)/data \
		--verbosity    3

## start-mem — run without persisting (fresh chain every time)
start-mem: build
	@echo ">>> Starting Thor solo node (in-memory, no persistence)..."
	$(THOR) solo \
		--eth-rpc-addr localhost:$(ETH_PORT) \
		--api-addr     localhost:$(API_PORT) \
		--verbosity    3

## install — npm install inside hardhat/
install:
	@cd $(ROOT)/hardhat && npm install

## deploy — deploy Counter (idempotent; skips if already deployed)
deploy: install
	@cd $(ROOT)/hardhat && npx hardhat run scripts/deploy.js --network vechain_solo

## increment — call Counter.increment() once
increment:
	@cd $(ROOT)/hardhat && npx hardhat run scripts/increment.js --network vechain_solo

## dapp — serve the DApp on http://localhost:$(DAPP_PORT)/dapp/
dapp:
	@echo ">>> DApp: http://localhost:$(DAPP_PORT)/dapp/"
	@echo "    (requires Counter deployed — run 'make deploy' first)"
	@cd $(ROOT) && python3 -m http.server $(DAPP_PORT)

## reset — delete persisted chain data and deployment record
reset:
	@rm -rf $(ROOT)/data
	@rm -f  $(ROOT)/hardhat/deployments/counter.json
	@echo ">>> Chain data and deployment removed."

## clean — reset + remove build artefacts and node_modules
clean: reset
	@rm -f  $(THOR)
	@rm -rf $(ROOT)/hardhat/node_modules
	@rm -rf $(ROOT)/hardhat/artifacts
	@rm -rf $(ROOT)/hardhat/cache
	@rm -rf $(ROOT)/hardhat/typechain-types
	@echo ">>> Clean done."
