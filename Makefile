SHELL = /bin/bash -o pipefail

# version (yyyymmdd)
DOCKER_IMAGE_VERSION = 20201103b
DOCKER_IMAGE_NAME = ghcr.io/llamerada-jp/colonio-buildenv
DOCKER_IMAGE = $(DOCKER_IMAGE_NAME):$(shell uname -m)-$(DOCKER_IMAGE_VERSION)

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
	ifeq ($(shell uname -m),x86_64)
	LIBWEBRTC_FILE = libwebrtc-86.0.4240.75-linux-amd64.tar.gz
	else ifeq ($(shell uname -m),aarch64)
	LIBWEBRTC_FILE = libwebrtc-86.0.4240.111-linux-arm64.tar.gz
	endif
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
	if [ $(shell uname -m) = 'x86_64' ]; then $(MAKE) setup-wasm; fi

.PHONY: setup-macos
setup-macos:
	mkdir -p $(WORK_PATH)
	brew update
	brew list > $(WORK_PATH)/BREW_PKGS
	install_pkgs="" && upgrade_pkgs="" \
	&& for p in autoconf automake cmake glog libtool libuv openssl pybind11; do \
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
		tar -zx -C $(shell realpath $(LOCAL_ENV_PATH)) -f $(LIBWEBRTC_FILE); \
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
	# Protocol Buffers
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
	# websocketpp
	cd $(WORK_PATH) \
	&& $(RM) -r websocketpp \
	&& git clone https://github.com/zaphoyd/websocketpp.git \
	&& cd websocketpp \
	&& git checkout refs/tags/$(WEBSOCKETPP_VERSION) \
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
	&& git clone https://github.com/protocolbuffers/protobuf.git protobuf_wasm \
	&& cd protobuf_wasm \
	&& git checkout refs/tags/v$(PROTOBUF_VERSION) \
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
	&& if [ $(shell uname -s) = 'Linux' ]; then cp $(LOCAL_ENV_PATH)/lib/lib*.so.* $(OUTPUT_PATH)/lib; fi

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
	&& $(RM) -r colonio-seed \
	&& git clone https://github.com/llamerada-jp/colonio-seed.git \
	&& LOCAL_ENV_PATH=$(LOCAL_ENV_PATH) $(MAKE) -C colonio-seed setup build \
	&& mkdir -p $(OUTPUT_PATH) \
	&& cp colonio-seed/seed $(OUTPUT_PATH)

.PHONY: build-docker-x86_64
build-docker-x86_64: $(ROOT_PATH)/buildenv/Makefile
	touch $(ROOT_PATH)/buildenv/dummy
	docker build $(ROOT_PATH)/buildenv/ -t $(DOCKER_IMAGE_NAME):x86_64-$(DOCKER_IMAGE_VERSION) --network host \
		--build-arg BASE_IMAGE=ubuntu:20.04 --build-arg QEMU_FILE=dummy

.PHONY: build-docker-aarch64
build-docker-aarch64: $(ROOT_PATH)/buildenv/Makefile
	sudo apt install qemu-user-static
	cp /usr/bin/qemu-aarch64-static $(ROOT_PATH)/buildenv/
	docker build $(ROOT_PATH)/buildenv/ -t $(DOCKER_IMAGE_NAME):aarch64-$(DOCKER_IMAGE_VERSION) --network host \
		--build-arg BASE_IMAGE=ubuntu@sha256:65cd340c0735f062e84507c3c2298502b5446037cc282bc1f3ac720ef42fb137 \
		--build-arg QEMU_FILE=qemu-aarch64-static

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
