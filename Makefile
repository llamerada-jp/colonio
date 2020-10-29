SHELL = /bin/bash

# paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
export LOCAL_ENV_PATH ?= $(ROOT_PATH)/local
export WORK_PATH ?= /tmp/work
BUILD_SEED_PATH := $(ROOT_PATH)/build
ifeq ($(shell uname -s),Darwin)
NATIVE_BUILD_PATH := $(ROOT_PATH)/build/macos_$(shell uname -m)
else ifeq ($(shell uname -s),Linux)
NATIVE_BUILD_PATH := $(ROOT_PATH)/build/linux_$(shell uname -m)
endif
OUTPUT_PATH :=$(ROOT_PATH)/output
WASM_BUILD_PATH := $(ROOT_PATH)/build/webassembly

# commands
export SUDO ?= sudo

# the versions of depending packages
ASIO_TAG = asio-1-12-2
CPP_ALGORITHMS_HASH = 5de21c513796a39f31e1db02a62fdb8dcc8ea775
EMSCRIPTEN_VERSION = 2.0.2
GTEST_VERSION = 1.10.0
LIBUV_VERSION = 1.12.0
ifeq ($(shell uname -s),Darwin)
LIBWEBRTC_FILE = libwebrtc-86.0.4240.80-macos-amd64.zip
else ifeq ($(shell uname -s),Linux)
LIBWEBRTC_FILE = libwebrtc-86.0.4240.75-linux-amd64.tar.gz
endif
LIBWEBRTC_VERSION = m86
PICOJSON_VERSION = 1.3.0
PROTOBUF_VERSION = 3.10.1
WEBSOCKETPP_VERSION = 0.8.1

# build options
BUILD_TYPE ?= Release
WITH_COVERAGE ?= OFF
WITH_SAMPLE ?= OFF
WITH_TEST ?= OFF

ifeq ($(shell uname -s),Darwin)
CMAKE_EXTRA_OPTS = -DOPENSSL_ROOT_DIR=$(shell brew --prefix openssl)
endif

.PHONY: all
all: build

.PHONY: setup
setup:
	if [ $(shell uname -s) = 'Linux' ]; then \
		$(MAKE) setup-linux; \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		$(MAKE) setup-macos; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: setup-linux
setup-linux:
	export DEBIAN_FRONTEND=noninteractive
	$(SUDO) apt update
	$(SUDO) apt -y install --no-install-recommends automake cmake build-essential ca-certificates curl git libcurl4-nss-dev libgoogle-glog-dev libssl-dev libtool pkg-config python3
	$(MAKE) setup-local

.PHONY: setup-macos
setup-macos:
	brew install autoconf automake cmake glog libtool libuv openssl pybind11
	$(MAKE) setup-local

.PHONY: setup-local
setup-local:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	# asio
	cd $(WORK_PATH) \
	&& $(RM) -r asio \
	&& git clone https://github.com/chriskohlhoff/asio.git \
	&& cd asio \
	&& git checkout refs/tags/$(ASIO_TAG) \
	&& cd asio \
	&& ./autogen.sh \
	&& ./configure --prefix=$(LOCAL_ENV_PATH) --without-boost \
	&& $(MAKE) \
	&& $(MAKE) install
	# cpp_algorithms
	cd $(WORK_PATH) \
	&& $(RM) -r cpp_algorithms \
	&& git clone https://github.com/hs-nazuna/cpp_algorithms.git \
	&& cd cpp_algorithms \
	&& git checkout $(CPP_ALGORITHMS_HASH) \
	&& cp DelaunayTriangulation/delaunay_triangulation.hpp $(LOCAL_ENV_PATH)/include/
	# emscripten
	cd $(LOCAL_ENV_PATH) \
	&& $(RM) -r emsdk \
	&& git clone https://github.com/emscripten-core/emsdk.git \
	&& cd emsdk \
	&& ./emsdk install $(EMSCRIPTEN_VERSION) \
	&& ./emsdk activate $(EMSCRIPTEN_VERSION)
	# gtest
	cd $(WORK_PATH) \
	&& $(RM) -r googletest \
	&& git clone https://github.com/google/googletest.git \
	&& cd googletest \
	&& git checkout refs/tags/release-$(GTEST_VERSION) \
	&& git submodule update --init --recursive \
	&& cmake -DCMAKE_INSTALL_PREFIX=$(LOCAL_ENV_PATH) . \
	&& $(MAKE) \
	&& $(MAKE) install
	# libuv
	cd $(WORK_PATH) \
	&& curl -LOS http://dist.libuv.org/dist/v$(LIBUV_VERSION)/libuv-v$(LIBUV_VERSION).tar.gz \
	&& tar zxf libuv-v$(LIBUV_VERSION).tar.gz \
	&& cd libuv-v$(LIBUV_VERSION) \
	&& sh autogen.sh \
	&& ./configure --prefix=$(LOCAL_ENV_PATH) \
	&& $(MAKE) \
	&& $(MAKE) install
	# libwebrtc
	cd $(WORK_PATH) \
	&& curl -LOS https://github.com/llamerada-jp/libwebrtc/releases/download/$(LIBWEBRTC_VERSION)/$(LIBWEBRTC_FILE) \
	&& if [ $(shell uname -s) = 'Linux' ]; then \
		tar zxf -C $(LOCAL_ENV_PATH) $(LIBWEBRTC_FILE); \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		unzip -o -d $(LOCAL_ENV_PATH) $(LIBWEBRTC_FILE); \
	fi
	# picojson
	cd $(WORK_PATH) \
	&& $(RM) -r picojson \
	&& git clone https://github.com/kazuho/picojson.git \
	&& cd picojson \
	&& git checkout refs/tags/v$(PICOJSON_VERSION) \
	&& cp picojson.h $(LOCAL_ENV_PATH)/include/
	# Protocol Buffers (native)
	cd $(WORK_PATH) \
	&& $(RM) -r protobuf \
	&& git clone https://github.com/protocolbuffers/protobuf.git \
	&& cd protobuf \
	&& git checkout refs/tags/v$(PROTOBUF_VERSION) \
	&& git submodule update --init --recursive \
	&& ./autogen.sh \
	&& ./configure --prefix=$(LOCAL_ENV_PATH) \
	&& $(MAKE) \
	&& $(MAKE) install
	# Protocol Buffers (wasm)
	cd $(WORK_PATH) \
	&& source $(LOCAL_ENV_PATH)/emsdk/emsdk_env.sh \
	&& mkdir -p em_cache \
	&& export EM_CACHE=$(WORK_PATH)/em_cache \
	&& $(RM) -r protobuf_wasm \
	&& git clone https://github.com/protocolbuffers/protobuf.git protobuf_wasm \
	&& cd protobuf_wasm \
	&& git checkout refs/tags/v$(PROTOBUF_VERSION) \
	&& git submodule update --init --recursive \
	&& ./autogen.sh \
	&& emconfigure ./configure --prefix=$(LOCAL_ENV_PATH)/wasm --disable-shared \
	&& emmake $(MAKE) \
	&& emmake $(MAKE) install
	# websocketpp
	cd $(WORK_PATH) \
	&& $(RM) -r websocketpp \
	&& git clone https://github.com/zaphoyd/websocketpp.git \
	&& cd websocketpp \
	&& git checkout refs/tags/$(WEBSOCKETPP_VERSION) \
	&& cmake -DCMAKE_INSTALL_PREFIX=$(LOCAL_ENV_PATH) . \
	&& $(MAKE) \
	&& $(MAKE) install

.PHONY: build
build:
	if [ $(shell uname -s) = 'Linux' ]; then \
		docker run -v $(ROOT_PATH):$(ROOT_PATH):rw \
			-u "$(shell id -u $(USER)):$(shell id -g $(USER))" \
			colonio-buildenv:$(shell uname -m) \
			-C $(ROOT_PATH) -j $(shell nproc) \
			build-native build-wasm \
			BUILD_TYPE=$(BUILD_TYPE) \
			WITH_COVERAGE=$(WITH_COVERAGE) \
			WITH_SAMPLE=$(WITH_SAMPLE) \
			WITH_TEST=$(WITH_TEST); \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		$(MAKE) build-native; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: test
test:
	LD_LIBRARY_PATH=$(OUTPUT_PATH)/lib $(MAKE) -C $(NATIVE_BUILD_PATH) test

.PHONY: build-native
build-native:
	mkdir -p $(NATIVE_BUILD_PATH) $(OUTPUT_PATH)/lib \
	&& cd $(NATIVE_BUILD_PATH) \
	&& PKG_CONFIG_PATH=$(LOCAL_ENV_PATH)/lib/pkgconfig/ \
		cmake -DLOCAL_ENV_PATH=$(LOCAL_ENV_PATH) \
		-DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
		-DCOLONIO_SEED_BIN_PATH=$(OUTPUT_PATH)/seed \
		-DWITH_COVERAGE=$(WITH_COVERAGE) \
		-DWITH_SAMPLE=$(WITH_SAMPLE) \
		-DWITH_TEST=$(WITH_TEST) \
		$(CMAKE_EXTRA_OPTS) \
		$(ROOT_PATH) \
	&& $(MAKE) \
	&& cp src/libcolonio.a $(OUTPUT_PATH) \
	&& cp $(LOCAL_ENV_PATH)/lib/lib*.so.* $(OUTPUT_PATH)/lib

.PHONY: build-wasm
build-wasm:
	mkdir -p $(WASM_BUILD_PATH) $(OUTPUT_PATH) \
	&& source $(LOCAL_ENV_PATH)/emsdk/emsdk_env.sh \
	&& mkdir -p /tmp/em_cache \
	&& export EM_CACHE=/tmp/em_cache \
	&& cd $(WASM_BUILD_PATH) \
	&& emcmake cmake -DLOCAL_ENV_PATH=$(LOCAL_ENV_PATH) -DCMAKE_BUILD_TYPE=$(BUILD_TYPE) $(ROOT_PATH) \
  && emmake $(MAKE) \
	&& cp src/colonio.* $(OUTPUT_PATH)

.PHONY: build-seed
build-seed:
	cd $(BUILD_SEED_PATH) \
	&& git clone https://github.com/llamerada-jp/colonio-seed.git \
	&& LOCAL_ENV_PATH=$(LOCAL_ENV_PATH) colonio-seed/build.sh \
	&& cp colonio-seed/seed $(OUTPUT_PATH)

.PHONY: build-docker
build-docker: $(ROOT_PATH)/buildenv/Makefile
	docker build $(ROOT_PATH)/buildenv -t colonio-buildenv:$(shell uname -m) --network host

$(ROOT_PATH)/buildenv/Makefile: $(ROOT_PATH)/Makefile
	cp Makefile buildenv

.PHONY: clean
clean:
	$(RM) -r $(OUTPUT_PATH)

.PHONY: deisclean
deisclean: clean
	$(RM) -r $(NATIVE_BUILD_PATH) $(WASM_BUILD_PATH) $(BUILD_SEED_PATH) $(ROOT_PATH)/buildenv/Makefile