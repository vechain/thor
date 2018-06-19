PACKAGE = github.com/vechain/thor

GIT_COMMIT = $(shell git --no-pager log --pretty="%h" -n 1)
FAKE_GOPATH = /tmp/thor-build-$(GIT_COMMIT)
export GOPATH = $(FAKE_GOPATH)

SRC_BASE = $(FAKE_GOPATH)/src/$(PACKAGE)


THOR_VERSION = `cat cmd/thor/VERSION`
THOR_TAG = `git tag -l --points-at HEAD`
DISCO_VERSION = `cat cmd/disco/VERSION`
DISCO_TAG = `git tag -l --points-at HEAD`
PACKAGES = `cd $(SRC_BASE) && go list ./... | grep -v '/vendor/'`

.PHONY: thor disco all clean test

thor: |$(SRC_BASE)
	@echo "building $@..."
	@cd $(SRC_BASE) && go build -v -i -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(THOR_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(THOR_TAG)" ./cmd/thor
	@echo "done. executable created at 'bin/$@'"

disco: |$(SRC_BASE)
	@echo "building $@..."
	@cd $(SRC_BASE) && go build -v -i -o $(CURDIR)/bin/$@ -ldflags "-X main.version=$(DISCO_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitTag=$(DISCO_TAG)" ./cmd/disco
	@echo "done. executable created at 'bin/$@'"

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

