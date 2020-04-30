#!/bin/bash

set -eu

readonly ASIO_VERSION='1.12.2'
readonly ASIO_VERSION_STR='101202'
readonly ASIO_VERSION_TAG='1-12-2'
readonly EMSCRIPTEN_VERSION='1.39.4'
readonly GTEST_VERSION='1.10.0'
readonly LIBUV_VERSION='1.12.0'
readonly PICOJSON_VERSION='1.3.0'
readonly PROTOBUF_VERSION='3.10.1'
readonly PROTOBUF_VERSION_STR='3010001'
readonly WEBSOCKETPP_VERSION='0.8.1'

readonly UNAME_S=$(uname -s)
readonly UNAME_P=$(uname -p)

# Set local environment path.
set_env_info() {
  readonly ROOT_PATH=$(cd $(dirname $0)/.. && pwd)

  # Build path and options.
  if [ ${TARGET} = 'wasm' ]; then
    readonly BUILD_PATH=${ROOT_PATH}/build/webassembly
  elif [ ${UNAME_S} = 'Darwin' ]; then
    readonly BUILD_PATH=${ROOT_PATH}/build/${UNAME_S}
  elif [ ${UNAME_S} = 'Linux' ]; then
    . /etc/os-release
    readonly BUILD_PATH=${ROOT_PATH}/build/${ID}_${VERSION_ID}_${UNAME_P}
    export CC=cc
    export CXX=c++
  else
    echo "Thank you for useing. But sorry, this platform is not supported yet."
    exit 1
  fi

  # Job number for make command.
  if ! [ -z ${TRAVIS+x} ]; then
    readonly JOB_COUNT=1
  elif [ ${UNAME_S} = 'Darwin' ]; then
    readonly JOB_COUNT=$(sysctl -n hw.logicalcpu)
  elif [ ${UNAME_S} = 'Linux' ]; then
    readonly JOB_COUNT=$(nproc)
  else
    exit 1
  fi

  export COLONIO_GIT_PATH=${ROOT_PATH}

  if [ -z "${LOCAL_ENV_PATH+x}" ] ; then
    export LOCAL_ENV_PATH=${ROOT_PATH}/local
  fi
  mkdir -p ${LOCAL_ENV_PATH}/include
  mkdir -p ${LOCAL_ENV_PATH}/opt
  mkdir -p ${LOCAL_ENV_PATH}/src
  mkdir -p ${LOCAL_ENV_PATH}/wasm
  export PKG_CONFIG_PATH=${LOCAL_ENV_PATH}/lib/pkgconfig/
}

# Install requirement packages for building native program.
setup_native() {
  if [ ${UNAME_S} = 'Darwin' ]; then
    req_pkg=''
    has_pkg=$(brew list)
    for p in \
      cmake\
      glog\
      libuv\
      pybind11
    do
      if echo "${has_pkg}" | grep ${p}; then
        :
      else
        req_pkg="${p} ${req_pkg}"
      fi
    done

    if [ "${req_pkg}" != '' ] ; then
      brew install ${req_pkg}
    fi

  elif [ ${UNAME_S} = 'Linux' ]; then
    if ! [ -v TRAVIS ]; then
      sudo apt-get install -y pkg-config automake cmake build-essential curl libcurl4-nss-dev libtool libx11-dev libgoogle-glog-dev
    fi

    setup_libuv
  fi

  setup_asio
  setup_picojson
  setup_protoc_native
  setup_websocketpp
  setup_webrtc
  if [ "${WITH_TEST}" = 'true' ]; then
    setup_gtest
  fi
}

setup_wasm() {
  setup_picojson
  setup_emscripten
  setup_protoc_wasm
}

# Setup Emscripten
setup_emscripten() {
  is_ok='false'
  if [ -e ${LOCAL_ENV_PATH}/opt/emsdk/upstream/emscripten/emcc ]; then
    set +e
    ${LOCAL_ENV_PATH}/opt/emsdk/upstream/emscripten/emcc -v 2>&1 | grep emcc | grep ${EMSCRIPTEN_VERSION}
    if [ $? = 0 ]; then
      echo "emscripten ${EMSCRIPTEN_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/opt/emsdk ]; then
      cd ${LOCAL_ENV_PATH}/opt/emsdk
      git checkout .
      git checkout master
      git pull
    else
      cd ${LOCAL_ENV_PATH}/opt
      git clone https://github.com/emscripten-core/emsdk.git
    fi
    cd ${LOCAL_ENV_PATH}/opt/emsdk
    ./emsdk install ${EMSCRIPTEN_VERSION}
    ./emsdk activate ${EMSCRIPTEN_VERSION}
  fi
  cd ${LOCAL_ENV_PATH}/opt/emsdk
  if [ ${UNAME_S} = 'Darwin' ]; then
    source ./emsdk_env.sh
  else
    . ./emsdk_env.sh
  fi
}

# Compile libuv.
setup_libuv() {
  is_ok='false'
  if [ ${UNAME_S} = 'Linux' ]; then
    set +e
    pkg-config --modversion libuv | grep -o ${LIBUV_VERSION}
    if [ $? = 0 ]; then
      echo "libuv ${LIBUV_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    cd ${LOCAL_ENV_PATH}/src
    if ! [ -e "libuv-v${LIBUV_VERSION}.tar.gz" ]; then
        curl -OL "http://dist.libuv.org/dist/v${LIBUV_VERSION}/libuv-v${LIBUV_VERSION}.tar.gz"
    fi
    if ! [ -e "libuv-v${LIBUV_VERSION}" ]; then
        tar zxf "libuv-v${LIBUV_VERSION}.tar.gz"
    fi
    cd "libuv-v${LIBUV_VERSION}"
    sh autogen.sh
    ./configure --prefix=${LOCAL_ENV_PATH}
    make -j ${JOB_COUNT}
    make install
  fi
}

# Download picojson.
setup_picojson() {
  if [ -e ${LOCAL_ENV_PATH}/src/picojson ]; then
    cd ${LOCAL_ENV_PATH}/src/picojson
    git checkout .
    git checkout master
    git pull
  else
    cd ${LOCAL_ENV_PATH}/src
    git clone https://github.com/kazuho/picojson.git
  fi
  cd ${LOCAL_ENV_PATH}/src/picojson
  git checkout refs/tags/v${PICOJSON_VERSION}
  cp ${LOCAL_ENV_PATH}/src/picojson/picojson.h ${LOCAL_ENV_PATH}/include/
}

# Download libwebrtc
setup_webrtc() {
  readonly WEBRTC_VER="m78"

  cd ${LOCAL_ENV_PATH}/src

  if [ ${UNAME_S} = 'Darwin' ]; then
    readonly WEBRTC_FILE="libwebrtc-78.0.3904.108-macosx-10.15.1.zip"
    if ! [ -e "${WEBRTC_FILE}" ]; then
      curl -OL https://github.com/llamerada-jp/libwebrtc/releases/download/${WEBRTC_VER}/${WEBRTC_FILE}
      cd ${LOCAL_ENV_PATH}
      rm -rf include/webrtc
      unzip -o src/${WEBRTC_FILE}
    fi

  elif [ ${UNAME_S} = 'Linux' ]; then
    readonly WEBRTC_FILE="libwebrtc-78.0.3904.108-ubuntu-18.04-x64.tar.gz"
    if ! [ -e "${WEBRTC_FILE}" ]; then
      curl -OL https://github.com/llamerada-jp/libwebrtc/releases/download/${WEBRTC_VER}/${WEBRTC_FILE}
      cd ${LOCAL_ENV_PATH}
      rm -rf include/webrtc
      tar zxf src/${WEBRTC_FILE}
    fi
  fi
}

setup_asio() {
  is_ok='false'
  if [ -e ${LOCAL_ENV_PATH}/include/asio/version.hpp ]; then
    set +e
    cat ${LOCAL_ENV_PATH}/include/asio/version.hpp | grep "ASIO_VERSION" | grep ${ASIO_VERSION_STR}
    if [ $? = 0 ]; then
      echo "asio ${ASIO_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/asio ]; then
      cd ${LOCAL_ENV_PATH}/src/asio
      git checkout .
      git checkout master
      git pull
    else
      cd ${LOCAL_ENV_PATH}/src/
      git clone https://github.com/chriskohlhoff/asio.git
    fi
    cd ${LOCAL_ENV_PATH}/src/asio
    git checkout "refs/tags/asio-${ASIO_VERSION_TAG}"
    cd asio
    ./autogen.sh
    ./configure --prefix=${LOCAL_ENV_PATH} --without-boost
    make -j ${JOB_COUNT}
    make install
  fi
}

# Download WebSocket++
setup_websocketpp() {
  is_ok='false'
  if [ -e ${LOCAL_ENV_PATH}/include/websocketpp/version.hpp ]; then
    set +e
    cat ${LOCAL_ENV_PATH}/include/websocketpp/version.hpp | grep "user_agent" | grep "${WEBSOCKETPP_VERSION}"
    if [ $? = 0 ]; then
      echo "websocketpp ${WEBSOCKETPP_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/websocketpp ]; then
        cd ${LOCAL_ENV_PATH}/src/websocketpp
        git checkout .
        git checkout master
        git pull
    else
        cd ${LOCAL_ENV_PATH}/src
        git clone https://github.com/zaphoyd/websocketpp.git
    fi
    cd ${LOCAL_ENV_PATH}/src/websocketpp
    git checkout "refs/tags/${WEBSOCKETPP_VERSION}"
    cmake -DCMAKE_INSTALL_PREFIX=${LOCAL_ENV_PATH} ${LOCAL_ENV_PATH}/src/websocketpp
    make -j ${JOB_COUNT}
    make install
  fi
}

# Build Protocol Buffers on native
setup_protoc_native() {
  is_ok='false'
  if [ -e "${LOCAL_ENV_PATH}/include/google/protobuf/port_def.inc" ]; then
    set +e
    cat "${LOCAL_ENV_PATH}/include/google/protobuf/port_def.inc" | grep "define PROTOBUF_VERSION" | grep ${PROTOBUF_VERSION_STR}
    if [ $? = 0 ]; then
      echo "protobuf ${PROTOBUF_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/protobuf_native ]; then
        cd ${LOCAL_ENV_PATH}/src/protobuf_native
        git checkout .
        git checkout master
        git pull
    else
        cd ${LOCAL_ENV_PATH}/src
        git clone https://github.com/protocolbuffers/protobuf.git protobuf_native
    fi
    cd ${LOCAL_ENV_PATH}/src/protobuf_native
    git checkout "refs/tags/v${PROTOBUF_VERSION}"
    git submodule update --init --recursive
    ./autogen.sh
    ./configure --prefix=${LOCAL_ENV_PATH}
    make -j ${JOB_COUNT}
    make install
  fi

  cd ${ROOT_PATH}
  ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/core/*.proto
  ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/map_paxos/*.proto
  ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/pubsub_2d/*.proto

  build_protoc
}

# Build Protocol Buffers for WebAssembly
setup_protoc_wasm() {
  setup_protoc_native

  is_ok='false'
  if [ -e "${LOCAL_ENV_PATH}/wasm/include/google/protobuf/port_def.inc" ]; then
    set +e
    cat "${LOCAL_ENV_PATH}/wasm/include/google/protobuf/port_def.inc" | grep "define PROTOBUF_VERSION" | grep ${PROTOBUF_VERSION_STR}
    if [ $? = 0 ]; then
      echo "protobuf ${PROTOBUF_VERSION} installed"
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/protobuf_wasm ]; then
      cd ${LOCAL_ENV_PATH}/src/protobuf_wasm
      git checkout .
      git checkout master
      git pull
    else
      cd ${LOCAL_ENV_PATH}/src
      git clone https://github.com/protocolbuffers/protobuf.git protobuf_wasm
    fi
    cd ${LOCAL_ENV_PATH}/src/protobuf_wasm
    git checkout refs/tags/v${PROTOBUF_VERSION}
    git submodule update --init --recursive
    ./autogen.sh
    emconfigure ./configure --prefix=${LOCAL_ENV_PATH}/wasm --disable-shared
    emmake make -j ${JOB_COUNT}
    emmake make install
  fi
}

setup_gtest() {
  is_ok='false'
  set +e
  pkg-config --modversion gtest | grep -o ${GTEST_VERSION}
  if [ $? = 0 ]; then
    echo "google test ${GTEST_VERSION} installed"
    is_ok='true'
  fi
  set -e
  if [ ${is_ok} = 'false' ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/googletest ]; then
      cd ${LOCAL_ENV_PATH}/src/googletest
      git checkout .
      git checkout master
      git pull
    else
      cd ${LOCAL_ENV_PATH}/src
      git clone https://github.com/google/googletest.git googletest
    fi
    cd ${LOCAL_ENV_PATH}/src/googletest
    git checkout "refs/tags/release-${GTEST_VERSION}"
    git submodule update --init --recursive
    cmake -DCMAKE_INSTALL_PREFIX=${LOCAL_ENV_PATH} .
    make -j ${JOB_COUNT}
    make install
  fi
}

setup_seed() {
  # Clone or pull the repogitory of seed.
  if [ -z "${COLONIO_SEED_GIT_PATH+x}" ] ; then
    if [ -e "${LOCAL_ENV_PATH}/src/colonio-seed" ] ; then
      cd ${LOCAL_ENV_PATH}/src/colonio-seed
      git pull
    else
      cd ${LOCAL_ENV_PATH}/src
      git clone https://github.com/colonio/colonio-seed.git
    fi
    readonly COLONIO_SEED_GIT_PATH=${LOCAL_ENV_PATH}/src/colonio-seed
  fi

  # Build program of seed and export path.
  ${COLONIO_SEED_GIT_PATH}/build.sh
  export COLONIO_SEED_BIN_PATH=${COLONIO_SEED_GIT_PATH}/seed
}

# Compile native programs.
build_native() {
  opt=''
  if [ ${WITH_SAMPLE} = 'true' ]; then
    opt="${opt} -DWITH_SAMPLE=ON"
  fi
  if [ ${WITH_TEST} = 'true' ]; then
    opt="${opt} -DWITH_TEST=ON -DCOLONIO_SEED_BIN_PATH=${COLONIO_SEED_BIN_PATH}"
    if [ "${ENABLE_COVERAGE}" = 'true' ]; then
      opt="${opt} -DWITH_COVERAGE=ON"
    fi
  fi
  mkdir -p ${BUILD_PATH}
  cd ${BUILD_PATH}
  cmake -DLOCAL_ENV_PATH=${LOCAL_ENV_PATH} -DCMAKE_BUILD_TYPE=${BUILD_TYPE} ${opt} ${ROOT_PATH}
  make -j ${JOB_COUNT}
}

build_protoc() {
  cd ${ROOT_PATH}
  ./local/bin/protoc --cpp_out src -I src src/core/*.proto
  ./local/bin/protoc --cpp_out src -I src src/map_paxos/*.proto
  ./local/bin/protoc --cpp_out src -I src src/pubsub_2d/*.proto
}

build_wasm() {
  mkdir -p ${BUILD_PATH}
  cd ${BUILD_PATH}
  emcmake cmake -DLOCAL_ENV_PATH=${LOCAL_ENV_PATH} -DCMAKE_BUILD_TYPE=${BUILD_TYPE} ${ROOT_PATH}
  emmake make -j ${JOB_COUNT}
}

show_usage() {
  echo "Usage: $1 [-cdhwst]" 1>&2
  echo "  -c : Build test with coverage.(Native only)" 1>&2
  echo "  -d : Set build type to debug mode." 1>&2
  echo "  -h : Show this help." 1>&2
  echo "  -w : Build webassembly module." 1>&2
  echo "  -s : Build with sample programs.(Native only)" 1>&2
  echo "  -t : Build test programs.(Native only)" 1>&2
}

# Default options.
TARGET='native'
ENABLE_DEBUG='false'
ENABLE_COVERAGE='false'
WITH_SAMPLE='false'
WITH_TEST='false'

# Decode options.
while getopts cdhstw OPT
do
  case $OPT in
    c)  ENABLE_COVERAGE='true'
      WITH_TEST='true'
      ;;
    d)  ENABLE_DEBUG='true'
      ;;
    h)  show_usage $0
      exit 0
      ;;
    s)  WITH_SAMPLE='true'
      ;;
    t)  WITH_TEST='true'
      ;;
    w)  TARGET='wasm'
      ;;
    \?) show_usage $0
      exit 1
      ;;
  esac
done
shift $((OPTIND - 1))

set -x

set_env_info

if [ "${ENABLE_DEBUG}" = 'true' ]; then
  readonly BUILD_TYPE='Debug'
else
  readonly BUILD_TYPE='Release'
fi

if [ "${TARGET}" = 'native' ]; then
  setup_native
  if [ "${WITH_TEST}" = 'true' ] ; then
    setup_seed
  fi
  build_native
else # wasm
  setup_wasm
  build_wasm
fi
