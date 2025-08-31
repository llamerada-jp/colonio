FROM ubuntu:24.04 AS build-env

ENV DEBIAN_FRONTEND=noninteractive
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
  && apt-get update && apt-get install -y --no-install-recommends \
    pkgconf libsdl2-dev libsdl2-image-dev libsdl2-ttf-dev curl ca-certificates iproute2 \
  && update-ca-certificates

WORKDIR /colonio
COPY work/* .
ENV LD_LIBRARY_PATH=/colonio
RUN ldconfig
