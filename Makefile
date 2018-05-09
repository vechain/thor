PACKAGE = github.com/vechain/thor
export GOPATH = $(CURDIR)/.build
SRC_BASE = $(GOPATH)/src/$(PACKAGE)
PACKAGES = `cd $(SRC_BASE) && go list ./... | grep -v '/vendor/'`

DATEVERSION=`date -u +%Y%m%d`
COMMIT=`git --no-pager log --pretty="%h" -n 1`

.PHONY: thor disco all clean test

thor: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o bin/thor -ldflags "-X main.version=${DATEVERSION} -X main.gitCommit=${COMMIT}" ./cmd/thor

disco: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o bin/disco -ldflags "-X main.version=${DATEVERSION} -X main.gitCommit=${COMMIT}" ./cmd/disco

$(SRC_BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

all: thor disco

clean:
	-rm -rf bin/*

test: |$(SRC_BASE)
	@go test -cover $(PACKAGES)