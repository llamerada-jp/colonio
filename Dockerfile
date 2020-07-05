
ARG PROTOC_VERSION="3.12.3"

# stage 1
FROM golang:1.14.4 as build

RUN apt-get update && \
  apt-get -y install git unzip build-essential autoconf libtool

WORKDIR /work
RUN git clone https://github.com/google/protobuf.git && \
  cd protobuf && \
  git checkout "v3.12.3" && \
  ./autogen.sh && \
  ./configure && \
  make -j8 && \
  make install && \
  ldconfig && \
  make clean && \
  go get github.com/golang/protobuf/protoc-gen-go

ADD . /work/seed
WORKDIR /work/seed
RUN for f in \
  core/core.proto \
  core/node_accessor_protocol.proto \
  core/seed_accessor_protocol.proto; \
  do \
  protoc -Iapi --go_out=pkg/seed api/${f}; \
  done && \
  cd /work/seed && \
  CGO_ENABLED=0 GO111MODULE=on go build -o seed

# stage 2
FROM scratch

COPY --from=build /work/seed/seed /seed
ENTRYPOINT [ "/seed" ]
