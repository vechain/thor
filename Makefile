PACKAGE = github.com/vechain/thor

FAKE_GOPATH_SUFFIX = $(shell [ -e ".fake_gopath_suffix" ] || date +%s > .fake_gopath_suffix; cat .fake_gopath_suffix)
FAKE_GOPATH = /tmp/thor-build-$(FAKE_GOPATH_SUFFIX)
export GOPATH = $(FAKE_GOPATH)

SRC_BASE = $(FAKE_GOPATH)/src/$(PACKAGE)

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
GIT_TAG = $(shell git tag -l --points-at HEAD)
THOR_VERSION = $(shell cat cmd/thor/VERSION)
DISCO_VERSION = $(shell cat cmd/disco/VERSION)

PACKAGES = `cd $(SRC_BASE) && go list ./... | grep -v '/vendor/'`

.PHONY: thor disco all clean test

thor: |$(SRC_BASE)
	@echo "building $@..."
	@cd $(SRC_BASE) && go build -v -i -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(THOR_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/thor
	@echo "done. executable created at 'bin/$@'"

disco: |$(SRC_BASE)
	@echo "building $@..."
	@cd $(SRC_BASE) && go build -v -i -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(DISCO_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(GIT_TAG)" ./cmd/disco
	@echo "done. executable created at 'bin/$@'"

dep: |$(SRC_BASE)
ifeq ($(shell command -v dep 2> /dev/null),)
	@git submodule update --init
else
	@cd $(SRC_BASE) && dep ensure -vendor-only
endif

$(SRC_BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

all: thor disco

clean:
	-rm -rf \
$(FAKE_GOPATH) \
$(CURDIR)/bin/thor \
$(CURDIR)/bin/disco 

test: |$(SRC_BASE)
	@cd $(SRC_BASE) && go test -cover $(PACKAGES)

