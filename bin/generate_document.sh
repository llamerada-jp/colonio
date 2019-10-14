#!/bin/bash

set -eu

# Set local environment path.
set_env_info() {
  readonly ROOT_PATH=$(cd $(dirname $0)/.. && pwd)

  if [ -z "${LOCAL_ENV_PATH+x}" ] ; then
    export LOCAL_ENV_PATH=${ROOT_PATH}/local
  fi
  mkdir -p ${LOCAL_ENV_PATH}/src
}

setup() {
  pip3 install jinja2 Pygments
  setup_mcss
}

setup_mcss() {
  if [ -e ${LOCAL_ENV_PATH}/src/mcss ]; then
    cd ${LOCAL_ENV_PATH}/src/mcss
    git fetch
  else
    cd ${LOCAL_ENV_PATH}/src
    git clone https://github.com/mosra/m.css.git mcss
  fi
}

generate() {
  rm -rf ${ROOT_PATH}/doc/
  python3 ${LOCAL_ENV_PATH}/src/mcss/documentation/doxygen.py ${ROOT_PATH}/Doxyfile-mcss
}

set_env_info
setup
generate