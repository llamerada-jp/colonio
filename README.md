[![Codacy Badge](https://api.codacy.com/project/badge/Grade/9fdd7ba69834413084cdc14cdb4eca55)](https://www.codacy.com/manual/llamerada-jp/colonio?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=llamerada-jp/colonio&amp;utm_campaign=Badge_Grade)
[![Travis-CI](https://travis-ci.org/llamerada-jp/colonio.svg?branch=master)](https://travis-ci.org/llamerada-jp/colonio/branches)
[![Coverage Status](https://coveralls.io/repos/github/llamerada-jp/colonio/badge.svg?branch=master)](https://coveralls.io/github/llamerada-jp/colonio?branch=master)

# Colonio : Library let it easy to use distributed systems

By using the distributed algorithms well, you can take away the load of the servers and create applications with high real-time performance and so on.
But there is little knowledge of distributed algorithms and we can not try it easily.
The purpose of the majority of engineers is to realize their own service, and the use of algorithms is a means.
It is interesting but not essence of work to think and implement difficult algorithms.
The purpose of Colonio is to make it more versatile and to make it easy for everyone to use distributed algorithms that are easy to use.

## More information

- The status of this project is experimental.
- Please see [website](https://www.colonio.dev/) to get more information.
- [colonio-seed](https://github.com/llamerada-jp/colonio-seed) is the seed program for colonio.
- [libwebrtc](https://github.com/llamerada-jp/libwebrtc) is a depending library to use WebRTC on native environment.

## How to build the node library

### Build for C/C++

```console
$ ./bin/build.sh
```

There is an output file below.

- `build/<path for each environment>/src/libcolonio.a`

### Build for JavaScript (WebAssembly)

```console
$ ./bin/build.sh -w
```

There are output files below.

- `build/webassembly/colonio.js`
- `build/webassembly/colonio.wasm`

Flags for build script are below.

```
Usage: ./bin/build.sh [-cdhwst]
  -c : Build test with coverage.(Native only)
  -d : Set build type to debug mode.
  -h : Show this help.
  -w : Build webassembly module.
  -s : Build with sample programs.(Native only)
  -t : Build test programs.(Native only)
```

## How to build a C/C++ program using colonio

```console
$ g++ -I<path to colonio>/src \
-L<path to libcolonio> -L<path to colonio>/local/lib \
-lcolonio -lwebrtc -lprotobuf -lpthread -lssl \
<your source code>
```

## License

Apache License 2.0
