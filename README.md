[![Codacy Badge](https://app.codacy.com/project/badge/Grade/f9a412edd15c4435bca3cc4fe161386f)](https://www.codacy.com/gh/llamerada-jp/colonio/dashboard?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=llamerada-jp/colonio&amp;utm_campaign=Badge_Grade)
[![CI](https://github.com/llamerada-jp/colonio/workflows/CI/badge.svg)](https://github.com/llamerada-jp/colonio/actions?query=workflow%3ACI)
[![Coverage Status](https://coveralls.io/repos/github/llamerada-jp/colonio/badge.svg?branch=main)](https://coveralls.io/github/llamerada-jp/colonio?branch=main)

# Colonio : Library let it easy to use distributed systems

By using the distributed algorithms well, you can take away the load of the servers and create applications with high real-time performance and so on.
But there is little knowledge of distributed algorithms and we can not try it easily.
The purpose of the majority of engineers is to realize their own service, and the use of algorithms is a means.
It is interesting but not essence of work to think and implement difficult algorithms.
The purpose of Colonio is to make it more versatile and to make it easy for everyone to use distributed algorithms that are easy to use.

## Requirement to build

- Go 1.23+
- gcc 11+
- npm

## How to build the node library

### Pre build library

Colonio using WebRTC via cgo. So you need to build native library before run colonio.
These are not required if you use colonio via WASM.

```console
$ make build-lib
```

Following libraries are required to build the application using colonio.

- output/libcolonio.a
- dep/lib/libwebrtc.a

### Run test

```console
$ make test
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
