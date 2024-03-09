SHELL := /bin/bash -o pipefail

# Tool and lib versions
# https://github.com/protocolbuffers/protobuf/releases
PROTOC_VERSION :=  25.2
PROTOC_GEN_GO_VERSION := $(shell awk '/google.golang.org\/protobuf/ {print substr($$2, 2)}' go.mod)

# Commands
CURL := curl -sSLf
SUDO := sudo

BINDIR := $(shell pwd)/bin
PROTOC := PATH=$(BINDIR) $(BINDIR)/protoc

all: seed

.PHONY: seed
seed: internal/proto/colonio.pb.go
	go build -o output/seed ./cmd/seed

internal/proto/colonio.pb.go: colonio.proto
	$(PROTOC) --go_out=module=github.com/llamerada-jp/colonio:. $<

.PHONY: format-code
format-code:
	go fmt ./...

.PHONY: setup
setup:
	$(SUDO) apt-get -y install --no-install-recommends unzip
	# protoc
	$(CURL) -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(BINDIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)

.PHONY: clean
clean:
	rm -rf $(BINDIR)
	rm -rf include