SHELL := /bin/bash -o pipefail

# Tool and lib versions
# https://github.com/protocolbuffers/protobuf/releases
PROTOC_VERSION :=  25.2
PROTOC_GEN_GO_VERSION := $(shell awk '/google.golang.org\/protobuf/ {print substr($$2, 2)}' go.mod)

# Paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
OUTPUT_PATH := $(ROOT_PATH)/output
LOCAL_ENV_PATH := $(ROOT_PATH)/local
WORK_PATH := /tmp/work

# Vserions
# https://github.com/llamerada-jp/libwebrtc
LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m119/libwebrtc-119-linux-amd64.tar.gz"
# https://github.com/kazuho/picojson
PICOJSON_VERSION := 1.3.0

# Commands
CURL := curl -sSLf
SUDO := sudo

BINDIR := $(shell pwd)/bin
PROTOC := PATH=$(BINDIR) $(BINDIR)/protoc

all: seed build-lib build-js

seed: internal/proto/colonio.pb.go $(wildcard ***.go)
	go build -o $(OUTPUT_PATH)/seed ./cmd/seed

internal/proto/colonio.pb.go: colonio.proto
	$(PROTOC) --go_out=module=github.com/llamerada-jp/colonio:. $<

test/dist/wasm_exec.js: $(shell go env GOROOT)/misc/wasm/wasm_exec.js
	cp $< $@

INCLUDE_FLAGS := -I$(LOCAL_ENV_PATH)/include -I$(LOCAL_ENV_PATH)/include/third_party/abseil-cpp
WEBRTC_DEFS := -DWEBRTC_LINUX=1 -DWEBRTC_POSIX=1
CXX_FLAGS := -std=c++17 -fvisibility=hidden -fvisibility-inlines-hidden -Wall $(INCLUDE_FLAGS) $(WEBRTC_DEFS)
build-lib: $(wildcard internal/c/*.cpp)  $(wildcard internal/c/*.h)  $(wildcard internal/c/*.hpp) 
	mkdir -p $(WORK_PATH)
	$(CXX) -c -o $(WORK_PATH)/webrtc_config.o $(CXX_FLAGS) internal/c/webrtc_config.cpp
	$(CXX) -c -o $(WORK_PATH)/webrtc_link.o $(CXX_FLAGS) internal/c/webrtc_link.cpp
	rm -f $(OUTPUT_PATH)/libcolonio.a
	ar rcs $(OUTPUT_PATH)/libcolonio.a $(WORK_PATH)/webrtc_config.o $(WORK_PATH)/webrtc_link.o

.PHONY: format-code
format-code: internal/proto/colonio.pb.go
	go fmt ./...

export COLONIO_TEST_CERT := $(shell pwd)/localhost.crt
export COLONIO_TEST_KEY := $(shell pwd)/localhost.key
.PHONY: test
test: build-lib build-test generate-cert test/dist/wasm_exec.js
	go test -v -count=1 -race ./config/...
	go test -v -count=1 -race ./seed/...
	go test -v -count=1 -race ./internal/...
	go run ./test/luncher/ -c test/luncher/seed.json

.PHONY: generate-cert
generate-cert:
	openssl req -x509 -out localhost.crt -keyout localhost.key \
  -newkey rsa:2048 -nodes -sha256 \
  -subj '/CN=localhost' -extensions EXT -config <( \
   printf "[dn]\nCN=localhost\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:localhost\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")

build-test: build-js $(wildcard ***.go)
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

build-js: $(wildcard src/*.ts)
	npm run build

.PHONY: setup
setup:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	# tools for typescript
	npm install
	# protoc
	$(CURL) -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-linux-x86_64.zip
	unzip -o protoc.zip bin/protoc 'include/*'
	rm -f protoc.zip
	GOBIN=$(BINDIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)
	# libwebrtc
	cd $(WORK_PATH) \
	&& curl -LOS $(LIBWEBRTC_URL) \
	&& if [ $(shell uname -s) = "Linux" ]; then \
		tar -zx -C $(shell realpath $(LOCAL_ENV_PATH)) -f $(shell basename $(LIBWEBRTC_URL)); \
	elif [ $(shell uname -s) = "Darwin" ]; then \
		unzip -o -d $(LOCAL_ENV_PATH) $(shell basename $(LIBWEBRTC_URL)); \
	fi
	# picojson
	cd $(WORK_PATH) \
	&& $(RM) -r picojson \
	&& git clone --depth=1 --branch v$(PICOJSON_VERSION) https://github.com/kazuho/picojson.git \
	&& cd picojson \
	&& cp picojson.h $(LOCAL_ENV_PATH)/include/

.PHONY: clean
clean:
	rm -rf $(BINDIR)
	git checkout $(BINDIR)
	rm -rf include