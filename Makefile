SHELL := /bin/bash -o pipefail

# Tool and lib versions
# https://github.com/bufbuild/buf/releases
BUF_VERSION := 1.60.0
# https://github.com/protocolbuffers/protobuf/releases
PROTOC_VERSION := 33.1
PROTOC_GEN_GO_VERSION := $(shell awk '/google.golang.org\/protobuf/ {print substr($$2, 2)}' go.mod)
CONNECT_GO_VERSION := $(shell awk '/connectrpc.com\/connect/ {print substr($$2, 2)}' go.mod)

# Paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
OUTPUT_PATH := $(ROOT_PATH)/output
DEPENDING_PKG_PATH := $(ROOT_PATH)/dep
BINDIR := $(DEPENDING_PKG_PATH)/bin

# Commands
BUF := PATH=$(BINDIR) $(BINDIR)/buf
CURL := curl -sSLf
PROTOC := PATH=$(BINDIR) $(BINDIR)/protoc
SUDO := sudo

.PHONY: generate
generate:
	$(BUF) generate --path ./api/

.PHONY: lint
lint:
	$(BUF) lint --path ./api/

.PHONY: format-code
format-code: $(shell find . -type f -name '*.go')
	go fmt ./...

.PHONY: test
export COLONIO_TEST_CERT := $(shell pwd)/localhost.crt
export COLONIO_TEST_KEY := $(shell pwd)/localhost.key
export COLONIO_COOKIE_SECRET_KEY_PAIR := "test"
test: build-js build-test
	# unit tests
	go test -v -count=1 -race -coverprofile=seed.covprofile ./seed/...
	go test -v -count=1 -race -coverprofile=internal.covprofile ./internal/...
	# e2e tests for native
	go test -v -count=1 -race -coverprofile=e2e.covprofile ./test/e2e/
	# e2e tests for wasm
	go run ./test/cmd/luncher/ -d ./test/dist -j ./output/

.PHONY: generate-cert
generate-cert:
	openssl req -x509 -out localhost.crt -keyout localhost.key \
  -newkey rsa:2048 -nodes -sha256 \
  -subj '/CN=localhost' -extensions EXT -config <( \
   printf "[dn]\nCN=localhost\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:localhost\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")

.PHONY: build-test
build-test: build-js test/dist/tests.txt generate-cert test/dist/wasm_exec.js

test/dist/wasm_exec.js: $(shell go env GOROOT)/lib/wasm/wasm_exec.js
	cp $< $@

test/dist/tests.txt: $(shell find . -type f -name '*.go')
	rm -f test/dist/tests/*.wasm
	for target in ./config ./internal; do \
		for dir in `find $$target -name '*.go' -printf '%h\n' | sort -u`; do \
		  if [ "$$(dirname $$dir)" = "." ]; then \
				outname="$$(basename $$dir).wasm"; \
			else \
				outname="$$(basename $$(dirname $$dir))_$$(basename $$dir).wasm"; \
			fi; \
			GOOS=js GOARCH=wasm go test -c -o ./test/dist/tests/$$outname $$dir; \
		done \
	done 
	ls test/dist/tests/ | grep \.wasm > test/dist/tests.txt

.PHONY: build-js
build-js: $(OUTPUT_PATH)/colonio.js

# TODO: Somehow the build runs every time🙁
$(OUTPUT_PATH)/colonio.js: $(shell find ./src -type f -name '*.ts')
	npm run build

.PHONY: setup
setup:
	mkdir -p $(DEPENDING_PKG_PATH)
	# tools for typescript
	npm install
	# protoc
	$(CURL) -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o -d $(DEPENDING_PKG_PATH) protoc.zip bin/protoc
	rm -f protoc.zip
	GOBIN=$(BINDIR) go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	GOBIN=$(BINDIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(BINDIR) go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v$(CONNECT_GO_VERSION)

.PHONY: clean
clean:
	rm -rf $(DEPENDING_PKG_PATH) $(OUTPUT_PATH) \
	  $(shell pwd)/localhost.crt $(shell pwd)/localhost.key \
		$(shell pwd)/src/*.d.ts $(shell pwd)/test/dist/tests/*.wasm \
		$(shell pwd)/test/dist/tests.txt $(shell pwd)/test/dist/wasm_exec.js
	$(MAKE) -C simulator clean
	git checkout $(OUTPUT_PATH)
