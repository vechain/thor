# MainDB v4 Migration Paths

## Introduction

The `v2.2.0` release introduces database and SQLite changes to improve performance and storage. This document outlines the possible
migration paths.

**Note:** The examples below assume you are operating a node on mainnet.

## Table of Contents

- [Blue / Green Deployment](#blue--green-deployment)
- [Sync in Parallel](#sync-in-parallel)
    - [1. Docker Migration](#1-docker-migration)
    - [2. Manual Migration](#2-manual-migration)
- [Install Latest Version](#install-latest-version)
    - [Using Docker](#using-docker)
    - [Install From Source](#install-from-source)

## Blue / Green Deployment

- For environments implementing a blue/green deployment strategy , starting a new node with the update image and allowing it to
  sync before a switching traffic is a seamless approach. Once synced, traffic can be directed towards to the new node, and the
  old node can be stopped.

## Sync in Parallel

- Syncing in parallel minimizes downtime but requires additional CPU, RAM and storage resources.

### 1. Docker Migration

For setups where Docker volumes are mapped to a location on the host machine.

**Note**: The examples assume the default data directory within the container is used. If a custom directory is configured,
adjustments to the examples are required.

For an existing node with a host instance directory of `/path/to/thor`:

```html
docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   -p 8669:8669 \
   -p 11235:11235 \
   --name <your-container-name> \
   vechain/thor:v2.1.4 --network main <additional-flags>
```

Start a new container with `v2.2.0`, without exposing the ports:

```html
docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   --name node-new \
   vechain/thor:v2.2.0 --network main <additional-flags>
```

- The `v2.1.4` node will continue to operate and write data to the directory `/path/to/thor/instance-39627e6be7ec1b4a-v3`, while the
  `v2.2.0` node will write the new databases to `/path/to/thor/instance-39627e6be7ec1b4a-v4`.
- Allow some time for the new node to sync.
- You can inspect the logs using `docker logs --tail 25 node-new`.
- After the new node is fully synced, stop both nodes and restart the original container with the updated image.

```html
docker stop node-new
docker rm node-new
docker stop <container-name>
docker rm <container-name>

docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   -p 8669:8669 \
   -p 11235:11235 \
   --name <your-container-name> \
   vechain/thor:v2.2.0 --network main <additional-flags>
```

- Confirm that the node is functioning as expected, before cleaning up the old databases:

```bash
rm -rf /path/to/thor/instance-39627e6be7ec1b4a-v3
```

### 2. Manual Migration

For nodes that installed from the source, follow the steps below:

- Assuming the old nodes was started with:

```html
/previous/executable/thor --network main <additional-flags>
```

- Build the new `thor` binary as outlined in [Install From Source](#install-from-source)

- Start the new node with different API, Metrics, Admin and P2P ports:

```html
./bin/thor --network main \
    --api-addr localhost:8668 \
    --metrics-addr localhost:2102 \
    --admin-addr localhost:2103 \
    --p2p-port 11222 \
    <additional-flags>
```

- The `v2.1.4` node will continue to operate and write data to the data directory under `/data/dir/instance-39627e6be7ec1b4a-v3`, while
  `v2.2.0` node writes to `/data/dir/instance-39627e6be7ec1b4a-v4`.
- Allow the new node to sync before switching traffic.

#### Stopping and Switching Nodes

##### 1. Get the PID of the new node:

```html
lsof -n -i:8668
```

##### 2. Stop the new node:

```html
kill <pid>
```

##### 3. Get the PID of the old node:

```html
lsof -n -i:8669
```

##### 4. Stop the old node:

```html
kill <pid>
```

##### 5. Restart the original node command with the new binary:

```html
/new/executable/thor --network main <additional-flags>
```

##### 6. Remove the old databases:

```bash
rm -rf /data/dir/instance-39627e6be7ec1b4a-v3
```

## Install Latest Version

### Using Docker

```bash
docker pull vechain/thor:v2.2.0
```

### Install From Source

- Clone the repository and checkout the `v2.2.0` tag:

```bash
git clone https://github.com/vechain/thor.git --branch v2.2.0 --depth 1
```

- Build the `thor` binary:

```bash
cd thor
make thor
```

- Verify the binary:

```bash
./bin/thor --version
```

- (Optional), Copy the binary to a location in your `$PATH`:

```bash
sudo cp ./bin/thor /usr/local/bin
```
