#!/usr/bin/env bash

set -eux

readonly ARCH=$(uname -m)
readonly OS=$(uname -s)

if [ "${OS}" = "Linux" ]; then
  # config apt to install google chrome
  sudo sh -c 'echo "deb http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google-chrome.list'
  sudo wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | sudo apt-key add -
  sudo apt update
  sudo apt -y install --no-install-recommends google-chrome-stable unzip

  # cleanup and setup
  make clean setup

  # check format
  make format-code
  diffs=$(git diff | wc -l)
  if [ "$diffs" -ne 0 ]; then
    git diff
    exit 1
  fi

  # check lisence
  ng=0
  for FILE in $(find . -type f); do
    if [[ $FILE =~ /\.git|node_modules/ ]]; then
      continue
    fi
    if ! [[ $FILE =~ .*\.(go|ts)$ ]] || [[ $FILE =~ .*\.pb\.go$ ]] ; then
      continue
    fi
    if ! grep -q "Apache License" $FILE; then
      echo "Lisence is not applied: $FILE"
      ng=1
    fi
  done
  if [ "$ng" -ne 0 ]; then
    exit 1
  fi

  make
  make test
  exit 0

elif [ "${OS}" = "Darwin" ]; then
  # macos x86_64
  make clean setup
  make
  make test
  exit 0
fi

echo "Unsupported environemnt. ARCH=${ARCH} OS=${OS}"
exit 1
