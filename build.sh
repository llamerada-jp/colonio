#!/bin/bash

set -eux

readonly GO_VERSION='1.13.3'
readonly PROTOC_VERSION='3.10.1'
readonly GO_PROTOBUF_VERSION='1.3.2'

readonly UNAME_S=$(uname -s)
readonly UNAME_P=$(uname -p)

# Set local environment path.
set_env_info() {
  # ROOT_PATH
  readonly ROOT_PATH=$(cd $(dirname $0) && pwd)

  # COLONIO_SEED_GIT_PATH
  export COLONIO_SEED_GIT_PATH=${ROOT_PATH}

  # LOCAL_ENV_PATH
  if [ -z "${LOCAL_ENV_PATH+x}" ] ; then
      export LOCAL_ENV_PATH=${ROOT_PATH}/local
  fi
  mkdir -p ${LOCAL_ENV_PATH}/bin
  mkdir -p ${LOCAL_ENV_PATH}/lib
  mkdir -p ${LOCAL_ENV_PATH}/src
  export PATH=${LOCAL_ENV_PATH}/bin:${PATH}
  export PKG_CONFIG_PATH=${LOCAL_ENV_PATH}/lib/pkgconfig/
}

# Check go command and install it.
# TODO: I want check version of golang.
setup_go() {
  if [ ${UNAME_S} = 'Darwin' ] ; then
    set +e
    type go
    if ! [ $? = 0 ] ; then
      brew install go
    fi
    set -e
    export GO=$(which go)
    export PATH=$(go env GOPATH)/bin:${PATH}

  elif [ ${UNAME_S} = 'Linux' ] ; then
    is_ok='false'
    if [ -e ${LOCAL_ENV_PATH}/go/bin/go ] ; then
      set +e
      ${LOCAL_ENV_PATH}/go/bin/go version | grep ${GO_VERSION}
      if [ $? = 0 ] ; then
        is_ok='true'
      fi
      set -e
    fi
    if [ ${is_ok} = 'false' ] ; then
      cd ${LOCAL_ENV_PATH}/src
      curl -L -O https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz
      cd ${LOCAL_ENV_PATH}
      tar vzxf ${LOCAL_ENV_PATH}/src/go${GO_VERSION}.linux-amd64.tar.gz
    fi

    export GO111MODULE=on
    export GOPATH=${LOCAL_ENV_PATH}/gopath
    export GOROOT=${LOCAL_ENV_PATH}/go
    mkdir -p ${GOPATH}
    export GO=${GOROOT}/bin/go
    export PATH=$(${GO} env GOPATH)/bin:${PATH}
  fi
}

setup_protobuf() {
  is_ok='false'
  if [ -e ${LOCAL_ENV_PATH}/bin/protoc ]; then
    set +e
    ${LOCAL_ENV_PATH}/bin/protoc --version | grep ${PROTOC_VERSION}
    if [ $? = 0 ] ; then
      is_ok='true'
    fi
    set -e
  fi
  if [ ${is_ok} = 'false' ] ; then
    fname=""
    url=""
    if [ ${UNAME_S} = 'Darwin' ] ; then
      fname="protoc-${PROTOC_VERSION}-osx-x86_64.zip"
      url="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${fname}"
    elif [ ${UNAME_S} = 'Linux' ] ; then
      if [ ${UNAME_P} = 'i386' ] ; then
        fname="protoc-${PROTOC_VERSION}-linux-x86_32.zip"
        url="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${fname}"
      elif [ ${UNAME_P} = 'x86_64' ] ; then
        fname="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
        url="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${fname}"
      fi
    fi

    if [ ${fname} = '' ] ; then
      echo "Thank you for useing. But sorry, this platform is not supported yet."
      exit 1
    fi

    cd ${LOCAL_ENV_PATH}/src
    curl -L -O ${url}
    cd ${LOCAL_ENV_PATH}
    unzip src/${fname}
  fi

  if [ -e ${LOCAL_ENV_PATH}/src/protobuf ]; then
    cd ${LOCAL_ENV_PATH}/src/protobuf
    git checkout .
    git checkout master
    git pull
  else
    cd ${LOCAL_ENV_PATH}/src
    git clone https://github.com/golang/protobuf
  fi
  cd ${LOCAL_ENV_PATH}/src/protobuf
  git checkout "refs/tags/v${GO_PROTOBUF_VERSION}"
  ${GO} install ./protoc-gen-go
}

build() {
  for f in \
    core/core.proto\
    core/node_accessor_protocol.proto\
    core/seed_accessor_protocol.proto
  do
    protoc -I${ROOT_PATH}/api --go_out=${ROOT_PATH}/pkg/seed ${ROOT_PATH}/api/${f}
  done
  # mv ${ROOT_PATH}/src/proto/core/* ${ROOT_PATH}/src/proto
  # cd ${ROOT_PATH}/src
  cd ${ROOT_PATH}
  GO111MODULE=on ${GO} build -o seed
}

# Main routine.
set_env_info
setup_go
setup_protobuf
build
