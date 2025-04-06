FROM ubuntu:24.04 AS build-env

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update \
  && apt -y install --no-install-recommends \
  pkgconf libsdl2-dev libsdl2-image-dev libsdl2-ttf-dev curl ca-certificates \
  && update-ca-certificates \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /colonio
COPY work/* .
ENV LD_LIBRARY_PATH=/colonio
RUN ldconfig
