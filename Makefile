PACKAGE = github.com/vechain/thor

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
GIT_TAG = $(shell git tag -l --points-at HEAD)
THOR_VERSION = $(shell cat cmd/thor/VERSION)
DISCO_VERSION = $(shell cat cmd/disco/VERSION)

PACKAGES = `go list ./... | grep -v '/vendor/'`

MAJOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f1)
MINOR = $(shell go version | cut -d' ' -f3 | cut -b 3- | cut -d. -f2)
export GO111MODULE=on

.PHONY: thor disco all clean test

help:
	@egrep -h '\s#@\s' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?#@ "}; {printf "\033[36m  %-30s\033[0m %s\n", $$1, $$2}'

thor:| go_version_check #@ Build the `thor` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(THOR_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/thor
	@echo "done. executable created at 'bin/$@'"

disco:| go_version_check #@ Build the `disco` executable
	@echo "building $@..."
	@go build -v -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(DISCO_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/disco
	@echo "done. executable created at 'bin/$@'"

dep:| go_version_check
	@go mod download

go_version_check:
	@if test $(MAJOR) -lt 1; then \
		echo "Go 1.19 or higher required"; \
		exit 1; \
	else \
		if test $(MAJOR) -eq 1 -a $(MINOR) -lt 19; then \
			echo "Go 1.19 or higher required"; \
			exit 1; \
		fi \
	fi

all: thor disco #@ Build the `thor` and `disco` executables

clean: #@ Clean the build artifacts
	-rm -rf \
$(CURDIR)/bin/thor \
$(CURDIR)/bin/disco

test:| go_version_check #@ Run the tests
	@go test -cover $(PACKAGES)

test-coverage:| go_version_check #@ Run the tests with coverage
	@go test -race -coverprofile=coverage.out -covermode=atomic $(PACKAGES)
	@go tool cover -html=coverage.out

lint_command_check:
	@command -v golangci-lint || (echo "golangci-lint not found, please install it from https://golangci-lint.run/usage/install/")

lint: | go_version_check lint_command_check #@ Run 'golangci-lint' on new code changes
	@golangci-lint run --new

lint-all: | go_version_check lint_command_check #@ Run 'golangci-lint' on the entire codebase
	@golangci-lint run
