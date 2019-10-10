#!/bin/bash

set -eu

# Get OS environment parameters.
set_platform_info() {
    if [ "$(uname -s)" = 'Darwin' ]; then
        # Mac OSX
        readonly ID='macos'
        readonly ARCH='x86_64'
        readonly IS_LINUX='false'

    elif [ -e /etc/os-release ]; then
        . /etc/os-release
        readonly ARCH=`uname -p`
        readonly IS_LINUX='true'

    else
        echo "Thank you for useing. But sorry, this platform is not supported yet."
        exit 1
    fi
}

# Set local environment path.
set_env_info() {
    readonly ROOT_PATH=$(cd $(dirname $0)/.. && pwd)
    if [ "${TARGET}" = 'web' ]; then
        readonly BUILD_PATH=${ROOT_PATH}/build/webassembly
    else
        readonly BUILD_PATH=${ROOT_PATH}/build/${ID}_${ARCH}
        if [ "${IS_LINUX}" = 'true' ]; then
            export CC=cc
            export CXX=c++
        fi
    fi

    if [ -z "${LOCAL_ENV_PATH+x}" ] ; then
        export LOCAL_ENV_PATH=${ROOT_PATH}/local
    fi
    mkdir -p ${LOCAL_ENV_PATH}/include
    mkdir -p ${LOCAL_ENV_PATH}/opt
    mkdir -p ${LOCAL_ENV_PATH}/src
    mkdir -p ${LOCAL_ENV_PATH}/wa
    export PKG_CONFIG_PATH=${LOCAL_ENV_PATH}/lib/pkgconfig/
}

# Install requirement packages for building native program.
setup_native() {
    if [ "${ID}" = 'macos' ]; then
        # cmake
        if brew list | grep cmake; then
            :
        else
            brew install cmake
        fi

        # libuv
        if brew list | grep libuv; then
            :
        else
            brew install libuv
        fi

        # pybind11
        if brew list | grep pybind11; then
            :
        else
            brew install pybind11
        fi

        # glog
        if brew list | grep glog; then
            :
        else
            brew install glog
        fi

        # asio
        if brew list | grep asio; then
            :
        else
            brew install asio
        fi

    elif type apt-get > /dev/null 2>&1; then
        if ! [ -v TRAVIS ]; then
            sudo apt-get install -y pkg-config automake cmake build-essential curl libcurl4-nss-dev libtool libx11-dev libgoogle-glog-dev libgtest-dev
        fi

        setup_asio

    else
        echo "Thank you for useing. But sorry, this platform is not supported yet."
        exit 1
    fi

    setup_libuv
    setup_picojson
    setup_websocketpp
    setup_protoc_native
    setup_webrtc
    if [ "${WITH_TEST}" = 'true' ]; then
        setup_gtest
    fi
}

setup_web() {
    setup_picojson
    setup_emscripten
    setup_protoc_web
}

# Setup Emscripten
setup_emscripten() {
    cd ${LOCAL_ENV_PATH}/src
    if ! [ -e emsdk-portable.tar.gz ]; then
        curl -OL https://s3.amazonaws.com/mozilla-games/emscripten/releases/emsdk-portable.tar.gz
    fi
    cd ${LOCAL_ENV_PATH}/opt
    if ! [ -e emsdk-portable ]; then
        tar vzxf ${LOCAL_ENV_PATH}/src/emsdk-portable.tar.gz
    fi
    cd ${LOCAL_ENV_PATH}/opt/emsdk-portable
    ./emsdk update
    ./emsdk install latest
    ./emsdk activate latest
    if [ "${ID}" = 'macos' ]; then
        source ./emsdk_env.sh
    else
        . ./emsdk_env.sh
    fi
}

# Compile libuv.
setup_libuv() {
    if [ "${ID}" != 'macos' ]; then
        if pkg-config --modversion libuv | grep -o 1.12.0 >/dev/null
        then
            echo libuv 1.12.0 installed
        else
            cd ${LOCAL_ENV_PATH}/src
            if ! [ -e libuv-v1.12.0.tar.gz ]; then
                wget http://dist.libuv.org/dist/v1.12.0/libuv-v1.12.0.tar.gz
            fi
            if ! [ -e libuv-v1.12.0 ]; then
                tar zxf libuv-v1.12.0.tar.gz
            fi
            cd libuv-v1.12.0
            sh autogen.sh
            ./configure --prefix=${LOCAL_ENV_PATH}
            make
            make install
        fi
    fi
}

# Download picojson.
setup_picojson() {
    if [ -e ${LOCAL_ENV_PATH}/src/picojson ]; then
        cd ${LOCAL_ENV_PATH}/src/picojson
        git fetch
    else
        cd ${LOCAL_ENV_PATH}/src
        git clone https://github.com/kazuho/picojson.git
    fi
    cd ${LOCAL_ENV_PATH}/src/picojson
    git checkout refs/tags/v1.3.0
    cp ${LOCAL_ENV_PATH}/src/picojson/picojson.h ${LOCAL_ENV_PATH}/include/
}

# Download libwebrtc
setup_webrtc() {
    cd ${LOCAL_ENV_PATH}/src
    if [ "${ID}" = 'macos' ]; then
        readonly WEBRTC_VER="m76"
        readonly WEBRTC_FILE="libwebrtc-76.0.3809.100-macosx-10.14.6.zip"
        
    else
        readonly WEBRTC_VER="m76"
        readonly WEBRTC_FILE="libwebrtc-76.0.3809.132-ubuntu-18.04-x64.tar.gz"
    fi

    if ! [ -e "${WEBRTC_FILE}" ]; then
        if [ "${ID}" = 'macos' ]; then
            curl -OL https://github.com/llamerada-jp/libwebrtc/releases/download/${WEBRTC_VER}/${WEBRTC_FILE}
            cd ${LOCAL_ENV_PATH}
            rm -rf include/webrtc
            unzip -o src/${WEBRTC_FILE}
        else
            wget https://github.com/llamerada-jp/libwebrtc/releases/download/${WEBRTC_VER}/${WEBRTC_FILE}
            cd ${LOCAL_ENV_PATH}
            rm -rf include/webrtc
            tar zxf src/${WEBRTC_FILE}
        fi
    fi
}

setup_asio() {
    if [ -e ${LOCAL_ENV_PATH}/src/asio ]; then
        cd ${LOCAL_ENV_PATH}/src/asio
        git fetch
    else
        cd ${LOCAL_ENV_PATH}/src/
        git clone https://github.com/chriskohlhoff/asio.git
    fi
    cd ${LOCAL_ENV_PATH}/src/asio
    git checkout refs/tags/asio-1-12-2
    cd asio
    ./autogen.sh
    ./configure --prefix=${LOCAL_ENV_PATH} --without-boost
    make
    make install
}

# Download WebSocket++
setup_websocketpp() {
    if [ -e ${LOCAL_ENV_PATH}/src/websocketpp ]; then
        cd ${LOCAL_ENV_PATH}/src/websocketpp
        git fetch
    else
        cd ${LOCAL_ENV_PATH}/src
        git clone https://github.com/zaphoyd/websocketpp.git
    fi
    cd ${LOCAL_ENV_PATH}/src/websocketpp
    git checkout refs/tags/0.8.1
    mkdir -p /tmp
    cmake -DCMAKE_INSTALL_PREFIX=${LOCAL_ENV_PATH} ${LOCAL_ENV_PATH}/src/websocketpp
    make install
}

# Build gtest
setup_gtest() {
    cd /usr/src/gtest
    sudo cmake .
    sudo cmake --build .
    sudo mv libg* /usr/local/lib/
}

# Build Protocol Buffers on native
setup_protoc_native() {
    if ! [ -e ${LOCAL_ENV_PATH}/bin/protoc ]; then
        if [ -e ${LOCAL_ENV_PATH}/src/protobuf_native ]; then
            cd ${LOCAL_ENV_PATH}/src/protobuf_native
            git fetch
        else
            cd ${LOCAL_ENV_PATH}/src
            git clone https://github.com/protocolbuffers/protobuf.git protobuf_native
        fi
            cd ${LOCAL_ENV_PATH}/src/protobuf_native
        git checkout refs/tags/v3.9.1
        git submodule update --init --recursive
        ./autogen.sh
        ./configure --prefix=${LOCAL_ENV_PATH}
        make
        make install
    fi

    cd ${ROOT_PATH}
    ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/core/*.proto
    ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/core/map_paxos/*.proto
    ${LOCAL_ENV_PATH}/bin/protoc -I=src --cpp_out=src src/core/pubsub_2d/*.proto

    build_protoc
}

# Build Protocol Buffers for WebAssembly
setup_protoc_web() {
    setup_protoc_native

    if [ -e ${LOCAL_ENV_PATH}/src/protobuf_wa ]; then
        cd ${LOCAL_ENV_PATH}/src/protobuf_wa
        git fetch
    else
        cd ${LOCAL_ENV_PATH}/src
        git clone https://github.com/protocolbuffers/protobuf.git protobuf_wa
    fi
    cd ${LOCAL_ENV_PATH}/src/protobuf_wa
    git checkout refs/tags/v3.9.1
    git submodule update --init --recursive
    ./autogen.sh
    emconfigure ./configure --prefix=${LOCAL_ENV_PATH}/wa --disable-shared
    emmake make
    emmake make install
}

# Compile native programs.
build_native() {
    if [ "${WITH_SAMPLE}" = 'true' ]; then
        OPT_SAMPLE='-DWITH_SAMPLE=ON'
    else
        OPT_SAMPLE=''
    fi
    if [ "${WITH_TEST}" = 'true' ]; then
        OPT_TEST='-DWITH_TEST=ON'
    else
        OPT_TEST=''
    fi
    mkdir -p ${BUILD_PATH}
    cd ${BUILD_PATH}
    cmake -DLOCAL_ENV_PATH=${LOCAL_ENV_PATH} -DCMAKE_BUILD_TYPE=${BUILD_TYPE} ${OPT_SAMPLE} ${OPT_TEST} ${ROOT_PATH}
    make
}

build_protoc() {
    cd ${ROOT_PATH}
    ./local/bin/protoc --cpp_out src -I src src/core/*.proto
    ./local/bin/protoc --cpp_out src -I src src/core/map_paxos/*.proto
    ./local/bin/protoc --cpp_out src -I src src/core/pubsub_2d/*.proto
}

build_web() {
    mkdir -p ${BUILD_PATH}
    cd ${BUILD_PATH}
    emcmake cmake -DLOCAL_ENV_PATH=${LOCAL_ENV_PATH} -DCMAKE_BUILD_TYPE=${BUILD_TYPE} ${ROOT_PATH}
    emmake make
}

show_usage() {
    echo "Usage: $1 [-dhw]" 1>&2
    echo "  -d : Set build type to debug mode." 1>&2
    echo "  -h : Show this help." 1>&2
    echo "  -w : Build webassembly module." 1>&2
    echo "  -s : Build with sample programs.(Native only)" 1>&2
    echo "  -t : Build test and coverage programs.(Native only)" 1>&2
}

# Default options.
TARGET='native'
ENABLE_DEBUG='false'
WITH_SAMPLE='false'
WITH_TEST='false'

# Decode options.
while getopts dhstw OPT
do

    case $OPT in
        d)  ENABLE_DEBUG='true'
            ;;
        h)  show_usage $0
            exit 0
            ;;
        s)  WITH_SAMPLE='true'
            ;;
        t)  WITH_TEST='true'
            ;;
        w)  TARGET='web'
            ;;
        \?) show_usage $0
            exit 1
            ;;
    esac
done
shift $((OPTIND - 1))

set -x

set_platform_info
set_env_info

if [ "${ENABLE_DEBUG}" = 'true' ]; then
    readonly BUILD_TYPE='Debug'
else
    readonly BUILD_TYPE='Release'
fi

if [ "${TARGET}" = 'native' ]; then
    setup_native
    build_native
else # web
    setup_web
    build_web
fi
