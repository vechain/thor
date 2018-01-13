PACKAGE = github.com/vechain/thor
TARGET = bin/thor
SYS_GOPATH := $(GOPATH)
export GOPATH = $(CURDIR)/.build
SRC_BASE = $(GOPATH)/src/$(PACKAGE)
PACKAGES = `cd $(SRC_BASE) && go list ./... | grep -v '/vendor/'`

DATEVERSION=`date -u +%Y%m%d`
COMMIT=`git --no-pager log --pretty="%h" -n 1`

.PHONY: thor
thor: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o $(TARGET) -ldflags "-X main.version=${DATEVERSION} -X main.gitCommit=${COMMIT}" ./cmd/thor/main.go

$(SRC_BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

.PHONY: install
install: thor
	@mv $(TARGET) $(SYS_GOPATH)/bin/

.PHONY: clean
clean:
	-rm -rf $(TARGET)


.PHONY: test
test: |$(SRC_BASE)
	@go test -cover $(PACKAGES)