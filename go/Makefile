# paths
ROOT_PATH := $(dir $(abspath $(lastword $(MAKEFILE_LIST))/..))
OUTPUT_PATH :=$(ROOT_PATH)/output


.PHONY: build_seed
build_seed:
	go build -o $(OUTPUT_PATH)/seed ./cmd/seed

.PHONY: test_native
test_native: build_seed
	COLONIO_SEED_BIN_PATH=$(OUTPUT_PATH)/seed CGO_LDFLAGS="-L$(OUTPUT_PATH) -L$(OUTPUT_PATH)/lib" go test ./test/

.PHONY: test_wasm
test_wasm:
	cp $(shell go env GOROOT)/misc/wasm/wasm_exec.js ./test
	GOOS=js GOARCH=wasm ginkgo build ./test/
	mv ./test/test.test ./test/test.wasm

.PHONY: setup
setup:
	go install github.com/onsi/ginkgo/ginkgo@latest
	