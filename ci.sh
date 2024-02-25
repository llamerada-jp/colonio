#!/usr/bin/env bash

set -eux

readonly ARCH=$(uname -m)
readonly OS=$(uname -s)

if [ "${OS}" = "Linux" ]; then
  # config apt to install google chrome
  sudo sh -c 'echo "deb http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google-chrome.list'
  sudo wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | sudo apt-key add -
  sudo apt update
  sudo apt install google-chrome-stable

  # check format
  make format-code
  diffs=$(git diff | wc -l)
  if [ "$diffs" -ne 0 ]; then
    exit 1
  fi

  # check lisence
  files=$(find . -type f -name "*.go" ! -name "*.pb.go")
  for file in $files; do
    if ! grep -q "Apache License" $file; then
      echo "Lisence is not applied: $file"
      exit 1
    fi
  done

  make clean
  make
  exit 0

elif [ "${OS}" = "Darwin" ]; then
  # macos x86_64
  
  make clean
  make
  exit 0
fi

echo "Unsupported environemnt. ARCH=${ARCH} OS=${OS}"
exit 1
