SHELL = /bin/bash -o pipefail

# paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
export LOCAL_ENV_PATH ?= $(ROOT_PATH)/local
export WORK_PATH ?= /tmp/work

# commands
export SUDO ?= sudo

# the versions of depending packages
PROTOBUF_VERSION = 3.10.1
GO_PROTOBUF_VERSION = 1.3.2
ifeq ($(shell uname -s),Darwin)
PROTOBUF_FNAME = protoc-$(PROTOBUF_VERSION)-osx-x86_64.zip
else ifeq ($(shell uname -s),Linux)
	ifeq ($(shell uname -m),x86_64)
	PROTOBUF_FNAME = protoc-$(PROTOBUF_VERSION)-linux-x86_64.zip
	else ifeq ($(shell uname -m),aarch64)
	PROTOBUF_FNAME = protoc-$(PROTOBUF_VERSION)-linux-aarch_64.zip
	endif
endif

.PHONY: all
all: build

.PHONY: setup
setup:
	if [ $(shell uname -s) = 'Linux' ]; then \
		$(MAKE) setup-linux setup-local; \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		$(MAKE) setup-macos setup-local; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: setup-linux
setup-linux:
	export DEBIAN_FRONTEND=noninteractive \
	&& $(SUDO) apt update \
	&& $(SUDO) apt -y install golang-1.14 unzip

.PHONY: setup-macos
setup-macos:
	mkdir -p $(WORK_PATH)
	brew list > $(WORK_PATH)/brew_pkgs
	brew update
	cat $(WORK_PATH)/brew_pkgs
	if grep ^go$$ $(WORK_PATH)/brew_pkgs; then \
		brew upgrade go; \
	else \
		brew install go; \
	fi

.PHONY: setup-local
setup-local:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	# Protocol Buffers
	cd $(WORK_PATH) \
	&& curl -LOS https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOBUF_VERSION)/$(PROTOBUF_FNAME) \
	&& cd $(LOCAL_ENV_PATH) \
	&& unzip -o $(WORK_PATH)/$(PROTOBUF_FNAME)
	# Protocol Buffers go
	cd $(WORK_PATH) \
	&& $(RM) -r protobuf \
	&& git clone https://github.com/golang/protobuf.git \
	&& cd protobuf \
	&& git checkout refs/tags/v$(GO_PROTOBUF_VERSION) \
	&& export GO111MODULE=on \
	&& export GOPATH=$(LOCAL_ENV_PATH) \
	&& unset GOROOT \
	&& export "PATH=/usr/lib/go-1.14/bin:$(PATH)" \
	&& go install ./protoc-gen-go

.PHONY: build
PROTOC = $(LOCAL_ENV_PATH)/bin/protoc
build:
	cd $(ROOT_PATH) \
	&& unset GOROOT \
	&& export PATH="/usr/lib/go-1.14/bin:$(LOCAL_ENV_PATH)/bin:$(PATH)" \
	&& $(PROTOC) -I$(ROOT_PATH)/api --go_out=$(ROOT_PATH)/pkg/seed $(ROOT_PATH)/api/core/core.proto \
	&& $(PROTOC) -I$(ROOT_PATH)/api --go_out=$(ROOT_PATH)/pkg/seed $(ROOT_PATH)/api/core/node_accessor_protocol.proto \
	&& $(PROTOC) -I$(ROOT_PATH)/api --go_out=$(ROOT_PATH)/pkg/seed $(ROOT_PATH)/api/core/seed_accessor_protocol.proto \
	&& GO111MODULE=on go build -o seed

.PHONY: clean
clean:
	$(RM) $(ROOT_PATH)/seed

.PHONY: deisclean
deisclean: clean
	$(RM) -r ${LOCAL_ENV_PATH} ${WORK_PATH}
