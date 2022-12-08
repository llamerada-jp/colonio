#!/usr/bin/env bash

set -eux

readonly ARCH=$(uname -m)
readonly OS=$(uname -s)

if [ "${OS}" = "Linux" ]; then
  if [ "${ARCH}" = "x86_64" ]; then
    # linux x86_64
    sudo apt install cmake clang-format valgrind
    pip3 install --user cpp-coveralls

    # check format
    make format-code src/core/colonio.pb.cc go/proto/colonio.pb.go
    diffs=$(git diff | wc -l)
    if [ "$diffs" -ne 0 ]; then
      exit 1
    fi

    make build BUILD_TYPE=Release
    make build BUILD_TYPE=Debug WITH_TEST=ON WITH_SAMPLE=ON WITH_COVERAGE=ON
    make setup-protoc
    make test CTEST_ARGS='--overwrite MemoryCheckCommandOptions="--leak-check=full" -T memcheck --output-on-failure'
    export PATH=$PATH:$(python3 -m site --user-base)/bin
    coveralls -b ./build/linux_x86_64/test/CMakeFiles/colonio_test.dir/__/ -i src -e src/test -E '.*\.pb\.h' -E '.*\.pb\.cc' --gcov-options '\-lp'
    exit 0

  elif [ "${ARCH}" = "aarch64" ]; then
    # linux aarch64
    sudo apt install cmake

    make build BUILT_TYPE=Release
    make build BUILT_TYPE=Debug WITH_TEST=ON
    make setup-protoc
    make test CTEST_ARGS='-v --timeout 300'
    exit 0
  fi

elif [ "${OS}" = "Darwin" ]; then
  # macos x86_64
  if [ -d ci_cache/local ]; then
    cp -a ci_cache/local local
    make setup SKIP_SETUP_LOCAL=ON
  else
    make setup
  fi

  make build BUILT_TYPE=Release
  make build BUILT_TYPE=Debug WITH_TEST=ON
  make test CTEST_ARGS='--output-on-failure'
  mkdir -p ci_cache
  cp -a local ci_cache/local
  exit 0
fi

echo "Unsupported environemnt. ARCH=${ARCH} OS=${OS}"
exit 1
