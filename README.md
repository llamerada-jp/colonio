[![Codacy Badge](https://api.codacy.com/project/badge/Grade/4b8bc767bd934017b5a17e172b511286)](https://app.codacy.com/manual/llamerada-jp/colonio-seed?utm_source=github.com&utm_medium=referral&utm_content=llamerada-jp/colonio-seed&utm_campaign=Badge_Grade_Dashboard)
[![Travis-CI](https://travis-ci.org/llamerada-jp/colonio-seed.svg?branch=master)](https://travis-ci.org/llamerada-jp/colonio-seed)

# Colonio seed

colonio-seed is seed/server program for [Colonio](https://github.com/llamerada-jp/colonio) node library.

Information about Colonio is on [this web page](https://www.colonio.dev/).

## Supported environments

- os
  - Ubuntu 18.04 or later
  - MacOS v10.14 (Mojave) or later
- golang 1.13 or later

## How to build

```sh
$ ./build.sh
```

## Usage

$ ./seed [flags] -config <config file>

| Flag     | Parameter type | Description                     |
| -------- | -------------- | ------------------------------- |
| -config  | string         | Specify the configuration file. |
| -syslog  |                | Output log using `syslog`.      |
| -verbose |                | Output verbose logs.            |

## License

Apache License 2.0
