# simulator

This simulation program uses a local Kubernetes setup to launch multiple nodes, to observe node network behavior.

## Components in the cluster

- **TURN server**: The TURN server using coTurn.
- **DB**: MongoDB to store the logs.
- **seed**: The colonio seed.
- **node**: A simple node that sends and receives messages to/from other nodes. And outputs the logs to DB.

## About viewer

## How to run the simulator

### Setup dependencies

```sh
make setup

#Install  k3s
curl -sfL https://get.k3s.io | sh -
# to use kubectl command  without sudo (optional)
# curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644

# Start k3s
sudo systemctl start k3s
```

### Deploy the simulator

```sh
make apply
# If you do not use k3s, please set KUBECTL variable.
# KUBECTL=kubectl make apply
```

### Export the logs

```sh
make export
```

### Stop simulation

```sh
make stop
# If you want to stop k3s
sudo systemctl stop k3s
# If you want to remove k3s
sudo k3s-uninstall.sh
```

### View the behavior of nodes with GUI

```sh
make view
# You can export to video file
make render
```