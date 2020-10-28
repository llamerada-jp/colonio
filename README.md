[![Codacy Badge](https://api.codacy.com/project/badge/Grade/9fdd7ba69834413084cdc14cdb4eca55)](https://www.codacy.com/manual/llamerada-jp/colonio?utm_source=github.com&utm_medium=referral&utm_content=llamerada-jp/colonio&utm_campaign=Badge_Grade)
[![Travis-CI](https://travis-ci.org/llamerada-jp/colonio.svg?branch=master)](https://travis-ci.org/llamerada-jp/colonio/branches)
[![Coverage Status](https://coveralls.io/repos/github/llamerada-jp/colonio/badge.svg?branch=master)](https://coveralls.io/github/llamerada-jp/colonio?branch=master)

# Colonio : Library let it easy to use distributed systems

By using the distributed algorithms well, you can take away the load of the servers and create applications with high real-time performance and so on.
But there is little knowledge of distributed algorithms and we can not try it easily.
The purpose of the majority of engineers is to realize their own service, and the use of algorithms is a means.
It is interesting but not essence of work to think and implement difficult algorithms.
The purpose of Colonio is to make it more versatile and to make it easy for everyone to use distributed algorithms that are easy to use.

## More information

-   The status of this project is experimental.
-   Please see [website](https://www.colonio.dev/) to get more information.
-   [colonio-seed](https://github.com/llamerada-jp/colonio-seed) is the seed program for colonio.
-   [libwebrtc](https://github.com/llamerada-jp/libwebrtc) is a depending library to use WebRTC on native environment.

## How to build the node library

### Build for C/C++ and JavaScript (WebAssembly)

```console
// with docker
$ make build

// without docker
$ make setup
$ make build-native build-wasm
```

There is an output file below.

-   `output/libcolonio.a`: static link library for C/C++
-   `output/lib/*`: depending shared library for C/C++
-   `output/colonio.*`: wasm library for JavaScript

### Run test

```console
$ make build WITH_TEST=ON
$ make build-seed
$ make test
```

Flags for build script are below.

| option          | values             | default   | description                                |
| --------------- | ------------------ | --------- | ------------------------------------------ |
| `BUILD_TYPE`    | `Release`, `Debug` | `Release` | build type option used as CMAKE_BUILD_TYPE |
| `WITH_TEST`     | `ON`, `OFF`        | `OFF`     | build test program                         |
| `WITH_COVERAGE` | `ON`, `OFF`        | `OFF`     | output coverage when run test programs     |

## How to build a C/C++ program using colonio

```console
$ g++ -I<path to colonio>/src \
-L<path to libcolonio> -L<path to colonio>/output \
-lcolonio -lwebrtc -lprotobuf -lpthread -lssl \
<your source code>
```

## License

Apache License 2.0
