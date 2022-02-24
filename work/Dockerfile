# stage 1
FROM golang:1.16 as build

COPY . /work
WORKDIR /work
RUN  apt update \
  && apt install -y --no-install-recommends build-essential=* \
  && make SUDO="" setup build

# stage 2
FROM scratch

COPY --from=build /work/bin/seed /seed
ENTRYPOINT [ "/seed" ]
