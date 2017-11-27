PACKAGE = github.com/vechain/vecore
TARGET = bin/vecore
SYS_GOPATH := $(GOPATH)
GOPATH = $(CURDIR)/.build
SRC_BASE = $(GOPATH)/src/$(PACKAGE)
PACKAGES = $(shell go list ./... | grep -v '/vendor/')

DATEVERSION=`date -u +%Y%m%d`
COMMIT=`git --no-pager log --pretty="%h" -n 1`

.PHONY: vecore
vecore: |$(SRC_BASE)
	@cd $(SRC_BASE) && go build -i -o $(TARGET) -ldflags "-X main.version=${DATEVERSION} -X main.gitCommit=${COMMIT}" ./cmd/vecore/main.go

$(SRC_BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

.PHONY: install
install: vcore
	@mv $(TARGET) $(SYS_GOPATH)/bin/

.PHONY: clean
clean:
	-rm -rf $(TARGET)


.PHONY: test
test: |$(SRC_BASE)
	@cd $(SRC_BASE) && go test $(PACKAGES)