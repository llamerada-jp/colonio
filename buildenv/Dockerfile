FROM --platform=$BUILDPLATFORM ubuntu:22.04 as prebuilder
LABEL org.opencontainers.image.source https://github.com/llamerada-jp/colonio

ENV DEBIAN_FRONTEND noninteractive
ENV GOPATH /go
ENV GOCACHE /go/.cache
ENV PATH $PATH:$GOPATH/bin:/opt/go/bin

RUN apt update \
  && apt -y install --no-install-recommends ca-certificates curl make tar valgrind unzip \
  && curl -L https://go.dev/dl/go1.21.6.linux-amd64.tar.gz | tar zx -C /opt

WORKDIR /work
COPY Makefile /work/
COPY go.mod /work/
COPY go.sum /work/
COPY package.json /work/
COPY package-lock.json /work/
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" \
  && chmod -R 777 "$GOPATH"
RUN LOCAL_ENV_PATH=/local SUDO="" WORK_PATH=/work make -C /work setup IN_DOCKER=ON -j "$(nproc)"\
  && chmod a+rx /local/bin/* \
  && rm -rf /work \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["make", "LOCAL_ENV_PATH=/local"]
