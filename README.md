[![CI](https://github.com/llamerada-jp/colonio/workflows/CI/badge.svg)](https://github.com/llamerada-jp/colonio/actions?query=workflow%3ACI)
[![Coverage Status](https://coveralls.io/repos/github/llamerada-jp/colonio/badge.svg?branch=main)](https://coveralls.io/github/llamerada-jp/colonio?branch=main)

# Colonio

Colonio is a peer-to-peer networking library that makes distributed algorithms easy to use.
It builds a WebRTC-based overlay network where nodes communicate directly with each other, reducing server load and enabling real-time applications.

## Features

- **Messaging** — Send a message to a specific node by its ID, with request/response support
- **Spread** — Broadcast a message to all nodes within a geographic area (geocast)
- **KVS** — Distributed key-value store with Raft consensus *(under development)*

Colonio also supports running nodes in the browser via WebAssembly.

## Architecture

Colonio consists of two components:

- **Seed** — A server that handles node registration and WebRTC signaling
- **Node** — A client that joins the P2P network and communicates with other nodes

After bootstrap, nodes establish direct WebRTC connections and relay packets through the overlay network.

For details, see [Architecture (ja)](doc/ja/architecture.adoc).

## Requirements

- Go 1.24+
- npm (for WebAssembly build)
- make
- zip

## Build and Test

```console
make test
```

## Documentation

- [Architecture (ja)](doc/ja/architecture.adoc)

## License

Apache License 2.0
