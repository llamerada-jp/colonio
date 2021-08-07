[![Codacy Badge](https://app.codacy.com/project/badge/Grade/95b9505c5cef4218ac548e4b60fc7638)](https://www.codacy.com/gh/llamerada-jp/colonio-seed/dashboard?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=llamerada-jp/colonio-seed&amp;utm_campaign=Badge_Grade)
[![test](https://github.com/llamerada-jp/colonio-seed/actions/workflows/ci.yaml/badge.svg)](https://github.com/llamerada-jp/colonio-seed/actions/workflows/ci.yaml)

# Colonio seed

colonio-seed is seed/server program for [Colonio](https://github.com/llamerada-jp/colonio) node library.

Information about Colonio is on [this web page](https://www.colonio.dev/).

## Supported environments

- os
  - Ubuntu 20.04 or later
  - MacOS v10.14 (Mojave) or later
- golang 1.13 or later

## How to build

```sh
$ make setup
$ make build
```

## Usage

$ ./seed [flags] -config <config file>

| Flag     | Parameter type | Description                     |
| -------- | -------------- | ------------------------------- |
| --config  | string         | Specify the configuration file. |

## License

Apache License 2.0
