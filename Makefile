PACKAGE = github.com/vechain/thor

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
GIT_TAG = $(shell git tag -l --points-at HEAD | head -n 1)
COPYRIGHT_YEAR = $(shell git --no-pager log -1 --format=%ad --date=format:%Y)
THOR_VERSION = $(shell cat cmd/thor/VERSION)
DISCO_VERSION = $(shell cat cmd/disco/VERSION)

PACKAGES = `go list ./... | grep -v '/vendor/'`
FUZZTIME=1m

REQUIRED_GO_MAJOR = 1
REQUIRED_GO_MINOR = 24
MAJOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f1)
MINOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f2)
export GO111MODULE=on

.DEFAULT_GOAL := thor
.PHONY: thor disco all clean test install-hooks

help:
	@egrep -h '\s#@\s' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?#@ "}; {printf "\033[36m  %-30s\033[0m %s\n", $$1, $$2}'

thor:| go_version_check #@ Build the `thor` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(THOR_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG) -X main.copyrightYear=$(COPYRIGHT_YEAR)" ./cmd/thor
	@echo "done. executable created at 'bin/$@'"

disco:| go_version_check #@ Build the `disco` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(DISCO_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG) -X main.copyrightYear=$(COPYRIGHT_YEAR)" ./cmd/disco
	@echo "done. executable created at 'bin/$@'"

dep:| go_version_check
	@go mod download

go_version_check:
	@if test $(MAJOR) -lt $(REQUIRED_GO_MAJOR); then \
		echo "Go $(REQUIRED_GO_MAJOR).$(REQUIRED_GO_MINOR) or higher required"; \
		exit 1; \
	else \
		if test $(MAJOR) -eq $(REQUIRED_GO_MAJOR) -a $(MINOR) -lt $(REQUIRED_GO_MINOR); then \
			echo "Go $(REQUIRED_GO_MAJOR).$(REQUIRED_GO_MINOR) or higher required"; \
			exit 1; \
		fi \
	fi

all: thor disco #@ Build the `thor` and `disco` executables

clean-bin: #@ Clean the bin folder
	-rm -rf \
$(CURDIR)/bin/thor \
$(CURDIR)/bin/disco

clean: #@ Clean the test and build cache and remove binaries
	@echo "cleaning test cache..."
	@go clean -testcache
	@echo "cleaning build cache and binaries..."
	@go clean -cache -modcache -i -r
	@rm -rf $(CURDIR)/bin/*
	@echo "done. build cache and binaries removed."

test:| go_version_check #@ Run the tests
	@go test -cover $(PACKAGES)

fuzz:| go_version_check #@ Run the fuzz tests
	@go test -fuzz=FuzzTransactionMarshalling -fuzztime=$(FUZZTIME) $(CURDIR)/tx
	@go test -fuzz=FuzzTransactionDecoding -fuzztime=$(FUZZTIME) $(CURDIR)/tx
	@go test -fuzz=FuzzReceiptDecoding -fuzztime=$(FUZZTIME) $(CURDIR)/tx
	@go test -fuzz=FuzzBlockEncoding -fuzztime=$(FUZZTIME) $(CURDIR)/block
	@go test -fuzz=FuzzHeaderEncoding -fuzztime=$(FUZZTIME) $(CURDIR)/block
	@go test -fuzz=FuzzBlockDecoding -fuzztime=$(FUZZTIME) $(CURDIR)/block

test-coverage:| go_version_check #@ Run the tests with coverage
	@go test -race -coverprofile=coverage.out -covermode=atomic $(PACKAGES)
	@go tool cover -html=coverage.out

lint_command_check:
	@command -v golangci-lint || (echo "golangci-lint not found, please install it from https://golangci-lint.run/usage/install/" && exit 1)

lint: | go_version_check lint_command_check #@ Run 'golangci-lint'
	@golangci-lint run --config .golangci.yml

license-check: #@ Check license headers
	@FILE_COUNT=$$(find . -type f -name '*.go' | wc -l); \
	echo "Checking license headers for all .go... $$FILE_COUNT files found"; \
	docker run -it --rm -v $$(pwd):/github/workspace apache/skywalking-eyes header check

install-hooks: #@ Install Git pre-commit hook
	@echo "▶ Installing Git hooks…"
	@mkdir -p .git/hooks
	@ln -sf $(CURDIR)/.git-hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git-hooks/pre-commit .git/hooks/pre-commit
	@echo "✅ Installed .git-hooks/pre-commit → .git/hooks/pre-commit"

.DEFAULT:
	@$(MAKE) help
