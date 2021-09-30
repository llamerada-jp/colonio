#!/usr/bin/env bash

set -eux

readonly ARCH=$(uname -m)
readonly OS=$(uname -s)

if [ "${OS}" = "Linux" ]; then
  if [ "${ARCH}" = "x86_64" ]; then
    # linux x86_64
    sudo apt install cmake
    pip3 install --user cpp-coveralls
    make build BUILD_TYPE=Release
    make build BUILD_TYPE=Debug WITH_TEST=ON WITH_COVERAGE=ON
    make build-seed
    make test
    export PATH=$PATH:$(python3 -m site --user-base)/bin
    coveralls -b ./build/linux_x86_64/test/CMakeFiles/colonio_test.dir/__/ -i src -e src/test -E '.*\.pb\.h' -E '.*\.pb\.cc' --gcov-options '\-lp'
    exit 0

  elif [ "${ARCH}" = "aarch64" ]; then
    # linux aarch64
    make build BUILT_TYPE=Release
    make build BUILT_TYPE=Debug WITH_TEST=ON
    make build-seed
    make test
    exit 0
  fi

elif [ "${OS}" = "Darwin" ]; then
  # macos x86_64
  make setup
  make build BUILT_TYPE=Release
  make build BUILT_TYPE=Debug WITH_TEST=ON
  make build-seed
  make test
  exit 0
fi

echo "Unsupported environemnt. ARCH=${ARCH} OS=${OS}"
exit 1
