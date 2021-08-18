SHELL := /bin/bash -o pipefail

# version (yyyymmdd)
DOCKER_IMAGE_VERSION := 20210409a
DOCKER_IMAGE_NAME := ghcr.io/llamerada-jp/colonio-buildenv
DOCKER_IMAGE := $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_VERSION)

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
ASIO_TAG := asio-1-18-1
CPP_ALGORITHMS_HASH := 5de21c513796a39f31e1db02a62fdb8dcc8ea775
EMSCRIPTEN_VERSION := 2.0.16
GTEST_VERSION := 1.10.0
LIBUV_VERSION := 1.41.0
ifeq ($(shell uname -s),Darwin)
LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m89/libwebrtc-89.0.4389.114-macos-amd64.zip"
else ifeq ($(shell uname -s),Linux)
	ifeq ($(shell uname -m),x86_64)
	LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m89.2/libwebrtc-89.0.4389.114-linux-amd64.tar.gz"
	else ifeq ($(shell uname -m),aarch64)
	LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m89/libwebrtc-89.0.4389.114-linux-arm64.tar.gz"
	endif
endif
PICOJSON_VERSION := 1.3.0
PROTOBUF_VERSION := 3.15.8
WEBSOCKETPP_VERSION := 0.8.2

# build options
BUILD_TYPE ?= Release
DESTDIR ?= /usr/local
WITH_COVERAGE ?= OFF
WITH_GPROF ?= OFF
WITH_PYTHON ?= ON
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
	$(SUDO) apt -y install --no-install-recommends automake cmake build-essential ca-certificates curl git libcurl4-nss-dev libgoogle-glog-dev libpython3-dev libssl-dev libtool pkg-config pybind11-dev python3 python3-distutils python3-pybind11
	$(MAKE) setup-local
	if [ $(shell uname -m) = 'x86_64' ]; then $(MAKE) setup-wasm; fi

.PHONY: setup-macos
setup-macos:
	mkdir -p $(WORK_PATH)
	brew update
	brew list > $(WORK_PATH)/BREW_PKGS
	install_pkgs="" && upgrade_pkgs="" \
	&& for p in autoconf automake cmake glog libtool libuv openssl pkg-config pybind11; do \
			if grep $${p} $(WORK_PATH)/BREW_PKGS; \
			then upgrade_pkgs="$${upgrade_pkgs} $${p}"; \
			else install_pkgs="$${install_pkgs} $${p}"; \
			fi \
		done \
	&& if [ "$${upgrade_pkgs}" != '' ]; then brew upgrade $${upgrade_pkgs}; fi \
	&& if [ "$${install_pkgs}" != '' ]; then brew install $${install_pkgs}; fi
	$(MAKE) setup-local

.PHONY: setup-local
setup-local:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	# asio
	cd $(WORK_PATH) \
	&& $(RM) -r asio \
	&& git clone --depth=1 --branch $(ASIO_TAG) https://github.com/chriskohlhoff/asio.git \
	&& cd asio/asio \
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
	# gtest
	cd $(WORK_PATH) \
	&& $(RM) -r googletest \
	&& git clone --depth=1 --branch release-$(GTEST_VERSION) https://github.com/google/googletest.git \
	&& cd googletest \
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
	&& curl -LOS $(LIBWEBRTC_URL) \
	&& if [ $(shell uname -s) = 'Linux' ]; then \
		tar -zx -C $(shell realpath $(LOCAL_ENV_PATH)) -f $(shell basename $(LIBWEBRTC_URL)); \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		unzip -o -d $(LOCAL_ENV_PATH) $(shell basename $(LIBWEBRTC_URL)); \
	fi
	# picojson
	cd $(WORK_PATH) \
	&& $(RM) -r picojson \
	&& git clone --depth=1 --branch v$(PICOJSON_VERSION) https://github.com/kazuho/picojson.git \
	&& cd picojson \
	&& cp picojson.h $(LOCAL_ENV_PATH)/include/
	# Protocol Buffers
	cd $(WORK_PATH) \
	&& $(RM) -r protobuf \
	&& git clone --depth=1 --branch v$(PROTOBUF_VERSION) https://github.com/protocolbuffers/protobuf.git \
	&& cd protobuf \
	&& git submodule update --init --recursive \
	&& ./autogen.sh \
	&& ./configure --prefix=$(LOCAL_ENV_PATH) \
	&& $(MAKE) \
	&& $(MAKE) install
	# websocketpp
	cd $(WORK_PATH) \
	&& $(RM) -r websocketpp \
	&& git clone --depth=1 --branch $(WEBSOCKETPP_VERSION) https://github.com/zaphoyd/websocketpp.git \
	&& cd websocketpp \
	&& cmake -DCMAKE_INSTALL_PREFIX=$(LOCAL_ENV_PATH) . \
	&& $(MAKE) \
	&& $(MAKE) install

.PHONY: setup-wasm
setup-wasm:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	# emscripten
	cd $(LOCAL_ENV_PATH) \
	&& $(RM) -r emsdk \
	&& git clone https://github.com/emscripten-core/emsdk.git \
	&& cd emsdk \
	&& ./emsdk install $(EMSCRIPTEN_VERSION) \
	&& ./emsdk activate $(EMSCRIPTEN_VERSION)
	# Protocol Buffers
	cd $(WORK_PATH) \
	&& source $(LOCAL_ENV_PATH)/emsdk/emsdk_env.sh \
	&& mkdir -p em_cache \
	&& export EM_CACHE=$(WORK_PATH)/em_cache \
	&& $(RM) -r protobuf_wasm \
	&& git clone --depth=1 --branch v$(PROTOBUF_VERSION) https://github.com/protocolbuffers/protobuf.git protobuf_wasm \
	&& cd protobuf_wasm \
	&& git submodule update --init --recursive \
	&& ./autogen.sh \
	&& emconfigure ./configure --prefix=$(LOCAL_ENV_PATH)/wasm --disable-shared \
	&& emmake $(MAKE) \
	&& emmake $(MAKE) install

.PHONY: build
ifeq ($(shell uname -m),x86_64)
BUILD_TARGET = build-native build-wasm
else ifeq ($(shell uname -m),aarch64)
BUILD_TARGET = build-native
endif
build:
	if [ $(shell uname -s) = 'Linux' ]; then \
		docker run -v $(ROOT_PATH):$(ROOT_PATH):rw \
			-u "$(shell id -u $(USER)):$(shell id -g $(USER))" \
			$(DOCKER_IMAGE) \
			-C $(ROOT_PATH) -j $(shell nproc) \
			$(BUILD_TARGET) \
			BUILD_TYPE=$(BUILD_TYPE) \
			WITH_COVERAGE=$(WITH_COVERAGE) \
			WITH_GPROF=$(WITH_GPROF) \
			WITH_PYTHON=$(WITH_PYTHON) \
			WITH_SAMPLE=$(WITH_SAMPLE) \
			WITH_TEST=$(WITH_TEST); \
	elif [ $(shell uname -s) = 'Darwin' ]; then \
		$(MAKE) build-native; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: test
test:
	# C/C++
	LD_LIBRARY_PATH=$(OUTPUT_PATH)/lib $(MAKE) -C $(NATIVE_BUILD_PATH) test
	# golang
	$(MAKE) -C go test_native
	
.PHONY: build-native
build-native:
	mkdir -p $(NATIVE_BUILD_PATH) $(OUTPUT_PATH)/lib \
	&& cd $(NATIVE_BUILD_PATH) \
	&& PKG_CONFIG_PATH=$(LOCAL_ENV_PATH)/lib/pkgconfig/ \
		cmake -DLOCAL_ENV_PATH=$(LOCAL_ENV_PATH) \
		-DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
		-DCMAKE_INSTALL_PREFIX=$(DESTDIR) \
		-DCOLONIO_SEED_BIN_PATH=$(OUTPUT_PATH)/seed \
		-DWITH_COVERAGE=$(WITH_COVERAGE) \
		-DWITH_GPROF=$(WITH_GPROF) \
		-DWITH_PYTHON=$(WITH_PYTHON) \
		-DWITH_SAMPLE=$(WITH_SAMPLE) \
		-DWITH_TEST=$(WITH_TEST) \
		$(CMAKE_EXTRA_OPTS) \
		$(ROOT_PATH) \
	&& $(MAKE) \
	&& cp src/libcolonio.a $(OUTPUT_PATH) \
	&& if [ $(shell uname -s) = 'Linux' ]; then cp $(LOCAL_ENV_PATH)/lib/lib*.so.* $(LOCAL_ENV_PATH)/lib/lib*.a $(OUTPUT_PATH)/lib; fi \
	&& if [ $(shell uname -s) = 'Darwin' ]; then cp $(LOCAL_ENV_PATH)/lib/lib*.a $(OUTPUT_PATH)/lib; fi

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
	$(MAKE) -C go build_seed

.PHONY: build-docker
build-docker: $(ROOT_PATH)/buildenv/Makefile
	docker buildx rm build-colonio || true
	docker buildx create --name build-colonio --platform linux/amd64,linux/arm/v7,linux/arm64/v8 --use
	docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64/v8 -t $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_VERSION) --push $(ROOT_PATH)/buildenv
	docker buildx rm build-colonio

$(ROOT_PATH)/buildenv/Makefile: $(ROOT_PATH)/Makefile
	cp Makefile $(ROOT_PATH)/buildenv

.PHONY: clean
clean:
	$(MAKE) -C $(NATIVE_BUILD_PATH) clean
	$(MAKE) -C $(WASM_BUILD_PATH) clean
	$(MAKE) -C $(BUILD_SEED_PATH)/colonio-seed clean
	find src -name *\.pb\.* -exec $(RM) {} \;
	$(RM) -r $(OUTPUT_PATH)

.PHONY: deisclean
deisclean: clean
	$(RM) -r $(NATIVE_BUILD_PATH) $(WASM_BUILD_PATH) $(BUILD_SEED_PATH)
	$(RM) -r $(ROOT_PATH)/buildenv/Makefile $(ROOT_PATH)/buildenv/dummy $(ROOT_PATH)/buildenv/qemu-aarch64-static
