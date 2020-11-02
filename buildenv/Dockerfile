ARG BASE_IMAGE
ARG QEMU_FILE

FROM ${BASE_IMAGE} as prebuilder
LABEL org.opencontainers.image.source https://github.com/llamerada-jp/colonio

COPY ${QEMU_FILE} /usr/bin/${QEMU_FILE}

ENV DEBIAN_FRONTEND noninteractive
RUN apt update \
  && apt -y install --no-install-recommends make

WORKDIR /work
COPY Makefile /work/
RUN LOCAL_ENV_PATH=/local SUDO= WORK_PATH=/work make -C /work setup -j $(nproc)\
  && rm -rf /work \
  && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["make", "LOCAL_ENV_PATH=/local"]
