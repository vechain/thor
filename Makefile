PACKAGE = github.com/vechain/thor
export GOPATH = $(CURDIR)/.build
SRC_BASE = $(GOPATH)/src/$(PACKAGE)
PACKAGES = `cd $(SRC_BASE) && go list ./... | grep -v '/vendor/'`

THOR_VERSION=`cat cmd/thor/VERSION`
THOR_TAG=`git tag -l --points-at HEAD`
DISCO_VERSION=`cat cmd/disco/VERSION`
DISCO_TAG=`git tag -l --points-at HEAD`

COMMIT=`git --no-pager log --pretty="%h" -n 1`

.PHONY: thor disco all clean test

thor: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o bin/thor -ldflags "-X main.version=${THOR_VERSION} -X main.gitCommit=${COMMIT} -X main.gitTag=${THOR_TAG}" ./cmd/thor

disco: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o bin/disco -ldflags "-X main.version=${DISCO_VERSION} -X main.gitCommit=${COMMIT} -X main.gitTag=${DISCO_TAG}" ./cmd/disco

$(SRC_BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

all: thor disco

clean:
	-rm -rf bin/*

test: |$(SRC_BASE)
	@go test -cover $(PACKAGES)