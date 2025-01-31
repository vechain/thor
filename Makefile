PACKAGE = github.com/vechain/thor

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
GIT_TAG = $(shell git tag -l --points-at HEAD | head -n 1)
COPYRIGHT_YEAR = $(shell git --no-pager log -1 --format=%ad --date=format:%Y)
THOR_VERSION = $(shell cat cmd/thor/VERSION)
DISCO_VERSION = $(shell cat cmd/disco/VERSION)

PACKAGES = $(shell go list ./... | grep -v '/vendor/' | grep -v 'github.com/vechain/thor/v2/api/fees')
# Space-separated list of packages to run tests sequentially
SEQUENTIAL_TEST_PACKAGES = github.com/vechain/thor/v2/api/fees 

MAJOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f1)
MINOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f2)
export GO111MODULE=on

.DEFAULT_GOAL := thor
.PHONY: thor disco all clean test

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
	@if test $(MAJOR) -lt 1; then \
		echo "Go 1.22 or higher required"; \
		exit 1; \
	else \
		if test $(MAJOR) -eq 1 -a $(MINOR) -lt 22; then \
			echo "Go 1.22 or higher required"; \
			exit 1; \
		fi \
	fi

all: thor disco #@ Build the `thor` and `disco` executables

clean-bins: #@ Clean the build artifacts
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

test-sequential:| go_version_check #@ Run the tests sequentially for SEQUENTIAL_TEST_PACKAGES
	@for pkg in $(SEQUENTIAL_TEST_PACKAGES); do \
    	echo "Running tests for $$pkg..."; \
    	go test -cover $$pkg || exit 1; \
	done

test:| go_version_check #@ Run the tests
	@$(MAKE) test-sequential
	@echo "Running tests for all other packages..."
	@go test -cover $(PACKAGES)

test-coverage-sequential:| go_version_check #@ Run the test coverage sequentially for SEQUENTIAL_TEST_PACKAGES
	@echo "mode: atomic" > coverage_seq.out
	@for pkg in $(SEQUENTIAL_TEST_PACKAGES); do \
    	echo "Running tests for $$pkg..."; \
    	go test -race -coverprofile=coverage_tmp.out -covermode=atomic $$pkg || exit 1; \
    	tail -n +2 coverage_tmp.out >> coverage_seq.out; \
	done
	@rm coverage_tmp.out

test-coverage:| go_version_check #@ Run the tests with coverage
	@$(MAKE) test-coverage-sequential
	@echo "Running tests with coverage for all other packages..."
	@go test -race -coverprofile=coverage_all.out -covermode=atomic $(PACKAGES)
	@echo "Combining coverage reports..."
	@echo "mode: atomic" > coverage.out && tail -n +2 coverage_seq.out >> coverage.out && tail -n +2 coverage_all.out >> coverage.out
	@rm coverage_seq.out coverage_all.out
	@go tool cover -html=coverage.out

lint_command_check:
	@command -v golangci-lint || (echo "golangci-lint not found, please install it from https://golangci-lint.run/usage/install/" && exit 1)

lint: | go_version_check lint_command_check #@ Run 'golangci-lint'
	@golangci-lint run --config .golangci.yml

license-check: #@ Check license headers
	@FILE_COUNT=$$(find . -type f -name '*.go' | wc -l); \
	echo "Checking license headers for all .go... $$FILE_COUNT files found"; \
	docker run -it --rm -v $$(pwd):/github/workspace apache/skywalking-eyes header check

.DEFAULT:
	@$(MAKE) help
