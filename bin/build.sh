#!/bin/bash

set -eux

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
  # ROOT_PATH
  readonly ROOT_PATH=$(cd $(dirname $0)/.. && pwd)

  # COLONIO_SEED_GIT_PATH
  export COLONIO_SEED_GIT_PATH=${ROOT_PATH}

  # LOCAL_ENV_PATH
  if [ -z "${LOCAL_ENV_PATH+x}" ] ; then
      export LOCAL_ENV_PATH=${ROOT_PATH}/local
  fi
  mkdir -p ${LOCAL_ENV_PATH}/bin
  mkdir -p ${LOCAL_ENV_PATH}/src
  export PATH=${LOCAL_ENV_PATH}/bin:${PATH}
  export PKG_CONFIG_PATH=${LOCAL_ENV_PATH}/lib/pkgconfig/
}

# Check go command and install it.
# TODO: I want check version of golang.
setup_go() {
  if [ "${ID}" = 'macos' ] ; then
    type go
    if ! [ $? = 0 ] ; then
      brew install go
    fi

  elif [ "${IS_LINUX}" = 'true' ] ; then
    if ! [ -e ${LOCAL_ENV_PATH}/go ] ; then
      cd ${LOCAL_ENV_PATH}/src
      wget https://dl.google.com/go/go1.13.4.linux-amd64.tar.gz
      cd ${LOCAL_ENV_PATH}
      tar vzxf ${LOCAL_ENV_PATH}/src/go1.13.4.linux-amd64.tar.gz
    fi
    export PATH=${LOCAL_ENV_PATH}/go/bin:${PATH}
  fi
}

setup_protobuf() {
  if ! [ -e ${LOCAL_ENV_PATH}/bin/protoc ]; then
    if [ -e ${LOCAL_ENV_PATH}/src/protobuf_native ]; then
      cd ${LOCAL_ENV_PATH}/src/protobuf_native
      git fetch
    else
      cd ${LOCAL_ENV_PATH}/src
      git clone https://github.com/protocolbuffers/protobuf.git protobuf_native
    fi
      cd ${LOCAL_ENV_PATH}/src/protobuf_native
     git checkout refs/tags/v3.10.1
    git submodule update --init --recursive
    ./autogen.sh
    ./configure --prefix=${LOCAL_ENV_PATH}
    make
    make install
  fi

  cd ${LOCAL_ENV_PATH}/src
  GIT_TAG="v1.3.2"

  if ! [ -e "$(go env GOPATH)/src/github.com/golang/protobuf/protoc-gen-go" ] ; then
    go get -d -u github.com/golang/protobuf/protoc-gen-go
  fi

  if [ -e "$(go env GOPATH)/src/github.com/golang/protobuf" ] ; then
    cd "$(go env GOPATH)/src/github.com/golang/protobuf"
    git fetch
    git checkout $GIT_TAG
  else
    git -C "$(go env GOPATH)"/src/github.com/golang/protobuf checkout $GIT_TAG
  fi

  go install github.com/golang/protobuf/protoc-gen-go
  export PATH=$(go env GOPATH)/bin:${PATH}
}

# Check COLONIO_GIT_PATH and clone, pull the repository if env is not exist.
setup_colonio() {
  if [ -z "${COLONIO_GIT_PATH+x}" ] ; then
    if [ -e "${LOCAL_ENV_PATH}/src/colonio" ] ; then
    cd ${LOCAL_ENV_PATH}/src/colonio
      git pull
    else
      cd ${LOCAL_ENV_PATH}/src
      git clone https://github.com/colonio/colonio.git
    fi
    readonly COLONIO_GIT_PATH=${LOCAL_ENV_PATH}/src/colonio
  fi
}

build() {
  for f in \
    core/node_accessor_protocol.proto\
    core/protocol.proto\
    core/seed_accessor_protocol.proto
  do
    protoc -I${COLONIO_GIT_PATH}/src --go_out=${ROOT_PATH}/src/proto ${COLONIO_GIT_PATH}/src/${f}
  done
  mv ${ROOT_PATH}/src/proto/core/* ${ROOT_PATH}/src/proto
  cd ${ROOT_PATH}/src
  GO111MODULE=on go build -o ${ROOT_PATH}/bin/seed main.go
}

# Main routine.
set_platform_info
set_env_info
setup_go
setup_protobuf
setup_colonio
build
