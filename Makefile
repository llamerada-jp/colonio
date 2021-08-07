SHELL = /bin/bash -o pipefail

# paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
export LOCAL_ENV_PATH ?= $(ROOT_PATH)/local
export WORK_PATH := /tmp/work

# commands
export SUDO := sudo
export PROTOC := $(LOCAL_ENV_PATH)/bin/protoc

# the versions of depending packages
PROTOBUF_VERSION := 3.15.8
GO_PROTOBUF_VERSION := 1.5.2
ifeq ($(shell uname -s),Darwin)
PROTOBUF_FNAME := protoc-$(PROTOBUF_VERSION)-osx-x86_64.zip
else ifeq ($(shell uname -s),Linux)
	ifeq ($(shell uname -m),x86_64)
	PROTOBUF_FNAME := protoc-$(PROTOBUF_VERSION)-linux-x86_64.zip
	else ifeq ($(shell uname -m),aarch64)
	PROTOBUF_FNAME := protoc-$(PROTOBUF_VERSION)-linux-aarch_64.zip
	endif
endif

.PHONY: all
all: build

.PHONY: setup
setup:
	if [ $(shell uname -s) = 'Linux' ]; then \
		$(MAKE) setup-linux setup-local; \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		$(MAKE) setup-local; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: setup-linux
setup-linux:
	export DEBIAN_FRONTEND=noninteractive \
	&& $(SUDO) apt update \
	&& $(SUDO) apt -y install cmake curl unzip

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
	&& git clone -b v$(GO_PROTOBUF_VERSION) --depth=1 https://github.com/golang/protobuf.git \
	&& cd protobuf \
	&& export GOPATH=$(LOCAL_ENV_PATH) \
	&& unset GOROOT \
	&& go install ./protoc-gen-go

.PHONY: build
build: seed/core/core.pb.go seed/core/node_accessor_protocol.pb.go seed/core/seed_accessor_protocol.pb.go
	mkdir -p bin
	go build -o bin/seed

seed/core/core.pb.go: api/core/core.proto
	PATH="$(LOCAL_ENV_PATH)/bin:$(PATH)" $(PROTOC) -I api --go_out=module=github.com/llamerada-jp/colonio-seed:. $<

seed/core/node_accessor_protocol.pb.go: api/core/node_accessor_protocol.proto
	PATH="$(LOCAL_ENV_PATH)/bin:$(PATH)" $(PROTOC) -I api --go_out=module=github.com/llamerada-jp/colonio-seed:. $<

seed/core/seed_accessor_protocol.pb.go: api/core/seed_accessor_protocol.proto
	PATH="$(LOCAL_ENV_PATH)/bin:$(PATH)" $(PROTOC) -I api --go_out=module=github.com/llamerada-jp/colonio-seed:. $<

.PHONY: clean
clean:
	$(RM) $(ROOT_PATH)/bin/seed

.PHONY: deisclean
deisclean: clean
	$(RM) -r ${LOCAL_ENV_PATH} ${WORK_PATH}
