# Colonio seed

colonio-seed is seed/server program for Colonio.

## How to build

```
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
