### Run test for browser

```console
# increase the maximum buffer size of UDP
# https://github.com/quic-go/quic-go/wiki/UDP-Receive-Buffer-Size
sysctl -w net.core.rmem_max=2500000

# start seed
go run ./go/cmd/seed/main.go -c test/js/seed.json
```

Access [https://localhost:8080/index.html](https://localhost:8080/index.html) using a browser and confirm that you get the `SUCCESS` message.

### Run test for Node.js

`TBD`