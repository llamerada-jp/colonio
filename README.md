[![Codacy Badge](https://app.codacy.com/project/badge/Grade/f9a412edd15c4435bca3cc4fe161386f)](https://www.codacy.com/gh/llamerada-jp/colonio/dashboard?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=llamerada-jp/colonio&amp;utm_campaign=Badge_Grade)
[![CI](https://github.com/llamerada-jp/colonio/workflows/CI/badge.svg)](https://github.com/llamerada-jp/colonio/actions?query=workflow%3ACI)
[![Coverage Status](https://coveralls.io/repos/github/llamerada-jp/colonio/badge.svg?branch=main)](https://coveralls.io/github/llamerada-jp/colonio?branch=main)

# Colonio : Library let it easy to use distributed systems

By using the distributed algorithms well, you can take away the load of the servers and create applications with high real-time performance and so on.
But there is little knowledge of distributed algorithms and we can not try it easily.
The purpose of the majority of engineers is to realize their own service, and the use of algorithms is a means.
It is interesting but not essence of work to think and implement difficult algorithms.
The purpose of Colonio is to make it more versatile and to make it easy for everyone to use distributed algorithms that are easy to use.

## More information

- The status of this project is experimental.
- Please see [website](https://www.colonio.dev/) to get more information.
- [libwebrtc](https://github.com/llamerada-jp/libwebrtc) is a depending library to use WebRTC on native environment.

## How to build the node library

### Build for C/C++ and JavaScript (WebAssembly)

```console
// for linux using docker
$ make build

// for mac or for linux without docker
$ make setup
$ make build-native build-wasm
```

There is an output file below.

- `output/libcolonio.a`: static link library for C/C++
- `output/lib/*`: depending shared library for C/C++
- `output/colonio.*`: wasm library for JavaScript

### Run test

```console
$ make build WITH_TEST=ON
$ make build-seed
$ make test [CTEST_ARGS=--verbose]
```

Flags for build script are below.

| option          | values             | default   | description                                |
| --------------- | ------------------ | --------- | ------------------------------------------ |
| `BUILD_TYPE`    | `Release`, `Debug` | `Release` | build type option used as CMAKE_BUILD_TYPE |
| `WITH_COVERAGE` | `ON`, `OFF`        | `OFF`     | output coverage when run test programs     |
| `WITH_GPROF`    | `ON`, `OFF`        | `OFF`     | enable build option for gprof              |
| `WITH_SAMPLE`   | `ON`, `OFF`        | `OFF`     | build sample programs                      |
| `WITH_TEST`     | `ON`, `OFF`        | `OFF`     | build test programs                        |

## How to build a C/C++ program using colonio

```console
$ g++ -I<path to colonio>/src \
-L<path to libcolonio> -L<path to colonio>/output \
-lcolonio -lwebrtc -lprotobuf -lpthread -lssl \
<your source code>
```

### Seed usage

```
$ ./seed [flags] -config <config file>
```

| Flag     | Parameter type | Description                     |
| -------- | -------------- | ------------------------------- |
| --config | string         | Specify the configuration file. |


## License

Apache License 2.0
