# simulator

This simulation program uses a local Kubernetes setup to launch multiple nodes, to observe node network behavior.

## Components in the cluster

- **DB**: MongoDB to store the logs.
- **seed**: The colonio seed.
- **node**: A simple node that sends and receives messages to/from other nodes. And outputs the logs to DB.

## About viewer

## How to run the simulator

### Setup dependencies

```sh
make setup download

# Install  k3s
# cf. https://docs.k3s.io/quick-start
curl -sfL https://get.k3s.io | sh -
# to use kubectl command  without sudo (optional)
# curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644

# Start k3s
sudo systemctl start k3s

# Install Chaos Mesh for k3s
# cf. https://chaos-mesh.org/docs/quick-start/
curl -sSL https://mirrors.chaos-mesh.org/v2.7.1/install.sh | bash -s -- --k3s
```

### Deploy the simulator

```sh
make simulate-random
# If you do not use k3s, please set KUBECTL variable.
# KUBECTL=kubectl make simulate-random

# Specify simulation story
# STORY=(circle|sphere)

# Specify the number of node pods
# NODES=3

# The command to simulate with a number of nodes proportional to the population in major cities and including delays based on real data is as follows
make STORY=sphere simulate-geo delay
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

# If you want to remove chaos-mesh
k3s kubectl delete ns chaos-mesh

# If you want to remove k3s
sudo k3s-uninstall.sh
```

### View the behavior of nodes with GUI

```sh
make view
# You can export to video file
make render
```