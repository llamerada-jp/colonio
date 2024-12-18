FROM ubuntu:24.04 AS build-env

WORKDIR /colonio
COPY work/* .
ENV LD_LIBRARY_PATH=/colonio
RUN ldconfig
