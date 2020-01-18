# Colonio seed

colonio-seed is seed/server program for Colonio.

Information about Colonio is on [this web page](https://www.colonio.dev/).

## Program status

* Code quality : [![Codacy Badge](https://api.codacy.com/project/badge/Grade/4b8bc767bd934017b5a17e172b511286)](https://app.codacy.com/manual/llamerada-jp/colonio-seed?utm_source=github.com&utm_medium=referral&utm_content=colonio/colonio-seed&utm_campaign=Badge_Grade_Dashboard)

## How to build

```sh
$ ./build.sh
```

## Flags for seed

| Flag     | Parameter type | Description                     |
| -------- | -------------- | ------------------------------- |
| -config  | string         | Specify the configuration file. |
| -syslog  |                | Output log using `syslog`.      |
| -verbose |                | Output verbose logs.            |

## Checked under

* macOS Catalina Version 10.15.2 + homebrew
* Ubuntu Bionic 18.04 (x86_64)
* Ubuntu Bionic 18.04 (WSL2)
* Ubuntu Xenial 16.04 (TravisCI)

## License

Apache License 2.0
