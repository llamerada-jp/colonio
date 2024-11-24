FROM ubuntu:24.04 AS build-env

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt -y install --no-install-recommends \
    pkgconf libsdl2{,-image,-ttf}-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /colonio
COPY work/* .
ENV LD_LIBRARY_PATH=/colonio
RUN ldconfig
