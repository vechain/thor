PACKAGE = github.com/vechain/thor

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
GIT_TAG = $(shell git tag -l --points-at HEAD)
THOR_VERSION = $(shell cat cmd/thor/VERSION)
DISCO_VERSION = $(shell cat cmd/disco/VERSION)

UNIT_TEST_PACKAGES = `go list ./... | grep -v '/vendor/'` | grep -v 'e2e'

MAJOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f1)
MINOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f2)
export GO111MODULE=on

.PHONY: thor disco all clean test

help: #@ Show a list of available commands
	@egrep -h '\s#@\s' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?#@ "}; {printf "\033[36m  %-30s\033[0m %s\n", $$1, $$2}'

thor:| go_version_check #@ Build the `thor` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(THOR_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/thor
	@echo "done. executable created at 'bin/$@'"

disco:| go_version_check #@ Build the `disco` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(DISCO_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/disco
	@echo "done. executable created at 'bin/$@'"

dep:| go_version_check #@ Download dependencies
	@go mod download

go_version_check: #@ Check go version
	@if test $(MAJOR) -lt 1; then \
		echo "Go 1.16 or higher required"; \
		exit 1; \
	else \
		if test $(MAJOR) -eq 1 -a $(MINOR) -lt 16; then \
			echo "Go 1.16 or higher required"; \
			exit 1; \
		fi \
	fi

all: thor disco #@ Build all executables

clean: #@ Clean the executables
	-rm -rf \
$(CURDIR)/bin/thor \
$(CURDIR)/bin/disco

test:| go_version_check #@ Run unit tests
	@go test -cover $(UNIT_TEST_PACKAGES)

test-e2e:| go_version_check #@ Run end-to-end tests
	GOMAXPROCS=1 go test github.com/vechain/thor/v2/tests/e2e

