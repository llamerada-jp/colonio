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

all: seed build-js

seed: internal/proto/colonio.pb.go $(wildcard ***.go)
	go build -o output/seed ./cmd/seed

internal/proto/colonio.pb.go: colonio.proto
	$(PROTOC) --go_out=module=github.com/llamerada-jp/colonio:. $<

test/dist/wasm_exec.js: $(shell go env GOROOT)/misc/wasm/wasm_exec.js
	cp $< $@

.PHONY: format-code
format-code: internal/proto/colonio.pb.go
	go fmt ./...

export COLONIO_TEST_CERT := $(shell pwd)/localhost.crt
export COLONIO_TEST_KEY := $(shell pwd)/localhost.key
.PHONY: test
test: build-test generate-cert test/dist/wasm_exec.js
	go test -v -count=1 ./seed/...
	go test -v -count=1 ./internal/...
	go run ./test/luncher/ -c test/luncher/seed.json

.PHONY: generate-cert
generate-cert:
	openssl req -x509 -out localhost.crt -keyout localhost.key \
  -newkey rsa:2048 -nodes -sha256 \
  -subj '/CN=localhost' -extensions EXT -config <( \
   printf "[dn]\nCN=localhost\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:localhost\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")

build-test: build-js $(wildcard ***.go)
	GOOS=js GOARCH=wasm go test -c -o ./test/dist/tests/network.wasm ./internal/network/

build-js: $(wildcard src/*.ts)
	npm run build

.PHONY: setup
setup:
	# tools for typescript
	npm install
	# protoc
	$(CURL) -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(BINDIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)

.PHONY: clean
clean:
	rm -rf $(BINDIR)
	git checkout $(BINDIR)
	rm -rf include