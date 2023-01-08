SHELL := /bin/bash -o pipefail

# version (yyyymmdd)
DOCKER_IMAGE_VERSION := 20230108a
DOCKER_IMAGE_NAME := ghcr.io/llamerada-jp/colonio-buildenv
DOCKER_IMAGE := $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_VERSION)

# paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
export LOCAL_ENV_PATH ?= $(ROOT_PATH)/local
export WORK_PATH ?= /tmp/work
ifeq ($(shell uname -s),Darwin)
NATIVE_BUILD_PATH := $(ROOT_PATH)/build/macos_$(shell uname -m)
else ifeq ($(shell uname -s),Linux)
NATIVE_BUILD_PATH := $(ROOT_PATH)/build/linux_$(shell uname -m)
endif
OUTPUT_PATH :=$(ROOT_PATH)/output
WASM_BUILD_PATH := $(ROOT_PATH)/build/webassembly

# commands
export SUDO ?= sudo
export PROTOC := $(LOCAL_ENV_PATH)/bin/protoc

# the versions of depending packages
# https://github.com/chriskohlhoff/asio/
ASIO_TAG := asio-1-24-0
# https://github.com/hs-nazuna/cpp_algorithms
CPP_ALGORITHMS_HASH := 1ba3fde9c4b1d067986f5243a0f03daffa501ae2
# https://github.com/emscripten-core/emscripten
EMSCRIPTEN_VERSION := 3.1.29
# https://github.com/google/googletest
GTEST_VERSION := 1.12.1
ifeq ($(shell uname -s),Darwin)
LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m108/libwebrtc-108.0.5359.124-macos-amd64.zip"
else ifeq ($(shell uname -s),Linux)
	ifeq ($(shell uname -m),x86_64)
	LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m108/libwebrtc-108.0.5359.124-linux-amd64.tar.gz"
	else ifeq ($(shell uname -m),aarch64)
	LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m108/libwebrtc-108.0.5359.124-linux-arm64.tar.gz"
	else ifeq ($(shell uname -m),arm)
	LIBWEBRTC_URL := "https://github.com/llamerada-jp/libwebrtc/releases/download/m108/libwebrtc-108.0.5359.124-linux-armhf.tar.gz"
	else
	exit 1
	endif
endif
# https://github.com/kazuho/picojson
PICOJSON_VERSION := 1.3.0
# https://github.com/protocolbuffers/protobuf
PROTOBUF_VERSION := 21.12
# https://github.com/golang/protobuf
GO_PROTOBUF_VERSION := 1.5.2
# https://github.com/zaphoyd/websocketpp
WEBSOCKETPP_VERSION := 0.8.2

# build options
BUILD_TYPE ?= Release
CTEST_ARGS ?= ""
DESTDIR ?= /usr/local
SKIP_SETUP_LOCAL ?= OFF
TEST_TIMEOUT ?= 180
WITH_COVERAGE ?= OFF
WITH_GPROF ?= OFF
WITH_SAMPLE ?= OFF
WITH_TEST ?= OFF
IN_DOCKER ?= OFF

ifeq ($(shell uname -s),Darwin)
CMAKE_EXTRA_OPTS = -DOPENSSL_ROOT_DIR=/usr/local/opt/openssl
endif

.PHONY: all
all: build

.PHONY: setup
setup:
	if [ $(shell uname -s) = "Linux" ]; then \
		$(MAKE) setup-linux; \
	elif [ $(shell uname -s) = "Darwin" ]; then \
		$(MAKE) setup-macos; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: setup-linux
setup-linux:
	export DEBIAN_FRONTEND=noninteractive
	if [ "$(SUDO)" = "" ]; then curl -fsSL "https://deb.nodesource.com/setup_18.x" | bash -; else curl -fsSL https://deb.nodesource.com/setup_18.x | $(SUDO) -E bash -; fi
	$(SUDO) apt update
	$(SUDO) apt -y install --no-install-recommends automake cmake build-essential ca-certificates curl git libcurl4-nss-dev libssl-dev libtool nodejs pkg-config python3
	if [ $(SKIP_SETUP_LOCAL) = "OFF" ]; then $(MAKE) setup-local; fi
	if [ $(SKIP_SETUP_LOCAL) = "OFF" -a $(shell uname -m) = "x86_64" ]; then $(MAKE) setup-wasm; fi

.PHONY: setup-macos
setup-macos:
	mkdir -p $(WORK_PATH)
	brew update
	brew list > $(WORK_PATH)/BREW_PKGS
	install_pkgs="" && upgrade_pkgs="" \
	&& for p in autoconf automake cmake libtool openssl@3 pkg-config pybind11; do \
			if grep "$${p}" "$(WORK_PATH)/BREW_PKGS"; \
			then upgrade_pkgs="$${upgrade_pkgs} $${p}"; \
			else install_pkgs="$${install_pkgs} $${p}"; \
			fi \
		done \
	&& if [ "$${upgrade_pkgs}" != "" ]; then brew upgrade $${upgrade_pkgs}; fi \
	&& if [ "$${install_pkgs}" != "" ]; then brew install $${install_pkgs}; fi \
	&& brew link --force openssl
	if [ $(SKIP_SETUP_LOCAL) = "OFF" ]; then $(MAKE) setup-local; fi

.PHONY: setup-local
setup-local:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
	$(MAKE) setup-protoc
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
	# websocketpp
	cd $(WORK_PATH) \
	&& $(RM) -r websocketpp \
	&& git clone --depth=1 --branch $(WEBSOCKETPP_VERSION) https://github.com/zaphoyd/websocketpp.git \
	&& cd websocketpp \
	&& cmake -DCMAKE_INSTALL_PREFIX=$(LOCAL_ENV_PATH) . \
	&& $(MAKE) \
	&& $(MAKE) install


.PHONY: setup-protoc
setup-protoc:
	mkdir -p $(LOCAL_ENV_PATH) $(WORK_PATH)
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
	# Protocol Buffers go
	cd $(WORK_PATH) \
	&& $(RM) -r protobuf \
	&& git clone -b v$(GO_PROTOBUF_VERSION) --depth=1 https://github.com/golang/protobuf.git \
	&& cd protobuf \
	&& export GOPATH=$(LOCAL_ENV_PATH) \
	&& unset GOROOT \
	&& go install ./protoc-gen-go

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
BUILD_TARGET = build-native build-wasm build-seed
else ifeq ($(shell uname -m),aarch64)
BUILD_TARGET = build-native build-seed
endif
build:
	if [ $(shell uname -s) = "Linux" ]; then \
		docker run \
			-v $(ROOT_PATH):$(ROOT_PATH):rw \
			-u "$(shell id -u $(USER)):$(shell id -g $(USER))" \
			--mount type=tmpfs,destination=/go \
			--mount type=tmpfs,destination=/.npm \
			--env GOCACHE=/go/.cache \
			$(DOCKER_IMAGE) \
			-C $(ROOT_PATH) -j $(shell nproc) \
			$(BUILD_TARGET) \
			IN_DOCKER=ON \
			BUILD_TYPE=$(BUILD_TYPE) \
			WITH_COVERAGE=$(WITH_COVERAGE) \
			WITH_GPROF=$(WITH_GPROF) \
			WITH_SAMPLE=$(WITH_SAMPLE) \
			WITH_TEST=$(WITH_TEST); \
	elif [ $(shell uname -s) = "Darwin" ]; then \
		$(MAKE) build-native build-seed; \
	else \
		echo "this platform is not supported yet."; \
	fi

.PHONY: test
test:
	# C/C++
	LD_LIBRARY_PATH=$(OUTPUT_PATH)/lib $(MAKE) -C $(NATIVE_BUILD_PATH) CTEST_OUTPUT_ON_FAILURE=1 test ARGS='$(CTEST_ARGS)'
	# golang
	$(MAKE) test-go-native

.PHONY: test-go-native
test-go-native: build-seed
	COLONIO_SEED_BIN_PATH=$(OUTPUT_PATH)/seed CGO_LDFLAGS="-L$(OUTPUT_PATH) -L$(OUTPUT_PATH)/lib" go test ./go/test/ -v

.PHONY: test-go-wasm
test-go-wasm: build-seed
	cp $(shell go env GOROOT)/misc/wasm/wasm_exec.js ./go/test
	GOOS=js GOARCH=wasm go test -c -o ./go/test/test.wasm ./go/test/
	$(OUTPUT_PATH)/seed -c $(ROOT_PATH)/go/test/seed.json

.PHONY: format-code
format-code:
	find {src,test} -name "*.cpp" -or -name "*.hpp" -exec clang-format -i {} \;
	docker run \
		-v $(ROOT_PATH):$(ROOT_PATH):rw \
		-u "$(shell id -u $(USER)):$(shell id -g $(USER))" \
		--mount type=tmpfs,destination=/go \
		--env GOCACHE=/go/.cache \
		$(DOCKER_IMAGE) \
		-C $(ROOT_PATH) \
		src/core/colonio.pb.cc \
		go/proto/colonio.pb.go

src/core/colonio.pb.cc: colonio.proto
	$(PROTOC) --cpp_out=src/core colonio.proto

.PHONY: build-native
build-native: src/core/colonio.pb.cc
	mkdir -p $(NATIVE_BUILD_PATH) $(OUTPUT_PATH)/lib \
	&& cd $(NATIVE_BUILD_PATH) \
	&& PKG_CONFIG_PATH=$(LOCAL_ENV_PATH)/lib/pkgconfig/ \
		cmake -DLOCAL_ENV_PATH=$(LOCAL_ENV_PATH) \
		-DTEST_TIMEOUT=$(TEST_TIMEOUT) \
		-DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
		-DCMAKE_INSTALL_PREFIX=$(DESTDIR) \
		-DCOLONIO_SEED_BIN_PATH=$(OUTPUT_PATH)/seed \
		-DWITH_COVERAGE=$(WITH_COVERAGE) \
		-DWITH_GPROF=$(WITH_GPROF) \
		-DWITH_SAMPLE=$(WITH_SAMPLE) \
		-DWITH_TEST=$(WITH_TEST) \
		$(CMAKE_EXTRA_OPTS) \
		$(ROOT_PATH) \
	&& $(MAKE) \
	&& cp src/libcolonio.a $(OUTPUT_PATH) \
	&& if [ $(shell uname -s) = "Linux" ]; then cp $(LOCAL_ENV_PATH)/lib/lib*.so.* $(LOCAL_ENV_PATH)/lib/lib*.a $(OUTPUT_PATH)/lib; fi \
	&& if [ $(shell uname -s) = "Darwin" ]; then cp $(LOCAL_ENV_PATH)/lib/lib*.a $(OUTPUT_PATH)/lib; fi

.PHONY: build-wasm
build-wasm: src/core/colonio.pb.cc
	mkdir -p $(WASM_BUILD_PATH) $(OUTPUT_PATH) \
	&& npm install \
	&& npm run build \
	&& source $(LOCAL_ENV_PATH)/emsdk/emsdk_env.sh \
	&& mkdir -p /tmp/em_cache \
	&& export EM_CACHE=/tmp/em_cache \
	&& cd $(WASM_BUILD_PATH) \
	&& emcmake cmake -DLOCAL_ENV_PATH=$(LOCAL_ENV_PATH) \
	    -DTEST_TIMEOUT=$(TEST_TIMEOUT) \
		-DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
		$(ROOT_PATH) \
  	&& emmake $(MAKE) \
	&& cp src/colonio.* $(OUTPUT_PATH)

.PHONY: build-seed
build-seed: go/proto/colonio.pb.go
	CGO_ENABLED=0 go build -o $(OUTPUT_PATH)/seed ./go/cmd/seed

go/proto/colonio.pb.go: colonio.proto
	PATH="$(LOCAL_ENV_PATH)/bin:$(PATH)" $(PROTOC) --go_out=module=github.com/llamerada-jp/colonio:. $<

.PHONY: build-docker
build-docker: $(ROOT_PATH)/buildenv/Makefile $(ROOT_PATH)/buildenv/go.mod $(ROOT_PATH)/buildenv/package.json
	docker buildx rm build-colonio || true
	docker buildx create --name build-colonio --platform linux/amd64,linux/arm/v7,linux/arm64/v8 --use
	docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64/v8 -t $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_VERSION) --push $(ROOT_PATH)/buildenv
	docker buildx rm build-colonio

$(ROOT_PATH)/buildenv/Makefile: $(ROOT_PATH)/Makefile
	cp Makefile $(ROOT_PATH)/buildenv/

$(ROOT_PATH)/buildenv/go.mod: $(ROOT_PATH)/go.mod
	cp go.mod go.sum $(ROOT_PATH)/buildenv/

$(ROOT_PATH)/buildenv/package.json: $(ROOT_PATH)/package.json
	cp package.json package-lock.json $(ROOT_PATH)/buildenv/

.PHONY: clean
clean:
	if [ -d $(NATIVE_BUILD_PATH) ]; then $(MAKE) -C $(NATIVE_BUILD_PATH) clean; fi
	if [ -d $(WASM_BUILD_PATH) ];   then $(MAKE) -C $(WASM_BUILD_PATH) clean;   fi
	find src -name *\.pb\.* -exec $(RM) {} \;
	$(RM) -r $(OUTPUT_PATH)

.PHONY: deisclean
deisclean: clean
	$(RM) -r ${LOCAL_ENV_PATH} $(NATIVE_BUILD_PATH) $(WASM_BUILD_PATH) $(BUILD_SEED_PATH) ${WORK_PATH}
	$(RM) -r $(ROOT_PATH)/buildenv/Makefile $(ROOT_PATH)/buildenv/dummy $(ROOT_PATH)/buildenv/qemu-aarch64-static
