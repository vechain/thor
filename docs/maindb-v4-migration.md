# MainDB v4 Migration

## Introduction

The `v2.2.0` release introduces database and SQLite changes to improve storage. This document outlines the possible
migration techniques.

Note that the examples below assume you are operating a node on mainnet.

## Table of Contents

- [Blue / Green Deployment](#blue--green-deployment)
- [Sync in Parallel](#sync-in-parallel)
  - [1. Docker Migration](#1-docker-migration)
  - [2. Manual Migration](#2-manual-migration)
- [Install Latest Version](#install-latest-version)
  - [Using Docker](#using-docker)
  - [Install From Source](#install-from-source)

## Blue / Green Deployment

- If you have implemented a blue/green deployment strategy, you can simply start a new node with the new image and
  allow it to sync. Once synced, you can switch the traffic to the new node and stop the old one.

## Sync in Parallel

- Syncing in parallel allows for minimal downtime.
- Note that syncing in parallel requires additional storage space. Assume more than double the size of the instance
  directory.

### 1. Docker Migration

Prior to running `v2.2.0`, you may have mapped the Docker volumes to a location on your host machine. 

**Note**: These examples assume you used the default data directory within the container. If you have used a different
directory, please adjust the examples accordingly.

Assuming you started your previous node with a host instance directory of `/path/to/thor`:

```html
docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   -p 8669:8669 \
   -p 11235:11235 \
   --name <your-container-name> \
   vechain/thor:v2.1.4 --network main <your-additional-flags>
```

With `v2.2.0`, you can start a new docker container without exposing the ports:

```html
docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   --name node-new \
   vechain/thor:v2.2.0 --network main <your-additional-flags>
```

- The `v2.1.4` node will continue to operate and write data to the directory `/path/to/thor/instance-39627e6be7ec1b4a-v3`, while
  `v2.2.0` will write the new databases to `/path/to/thor/instance-39627e6be7ec1b4a-v4`.
- Allow some time for the new node to sync. You can inspect the logs using `docker logs --tail 25 node-new`.
- Once the node is fully synced, it is time to switch the traffic to the new node.
- Stop both of the nodes.

```html
docker stop node-new
docker rm node-new
docker stop <your-container-name>
docker rm <your-container-name>
```

- Start the original command that you used **with the new image**, E.g.:

```html
docker run -d \
   -v /path/to/thor:/home/thor/.org.vechain.thor 
   -p 8669:8669 \
   -p 11235:11235 \
   --name <your-container-name> \
   vechain/thor:v2.2.0 --network main <your-additional-flags>
```

- Once you have confirmed that the node is functioning as expected, you can clean up the old databases:

```bash
rm -rf /path/to/thor/instance-39627e6be7ec1b4a-v3
```

### 2. Manual Migration

If you installed the `thor` CLI from the source, you can follow the steps below.

- Assuming you started the following command:

```html
/previous/executable/thor --network main <your-additional-flags>
```

- Follow the steps below to build the new `thor` binary: [Install From Source](#install-from-source)

- Start the new node. **Note** that it is important to run the node on different API, Metrics, and Admin addresses, as well as a different P2P port:

```html
./bin/thor --network main \
    --api-addr localhost:8668 \
    --metrics-addr localhost:2102 \
    --admin-addr localhost:2103 \
    --p2p-port 11222 \
    <your-additional-flags>
```

- The `v2.1.4` node will continue to operate and write data to the data directory under `/data/dir/instance-39627e6be7ec1b4a-v3`, while `v2.2.0` will write the new databases to `/data/dir/instance-39627e6be7ec1b4a-v4`.
- Allow some time for the new node to sync. 
- Once the node is fully synced, it is time to switch the traffic to the new node.
- Get the new nodes PID:

```html
lsof -n -i:8668
```

- Stop the new node:

```html
kill <pid>
```

- Get the old nodes PID:

```html
lsof -n -i:8669
```

- Stop the old node:

```html
kill <pid>
```

- Run the original command with the new binary:

```html
/new/executable/thor --network main <your-additional-flags>
```

## Install Latest Version

### Using Docker

```bash
docker pull vechain/thor:v2.2.0
```

### Install From Source

- Clone the repository and checkout the `v2.2.0` tag:

```bash
git clone --branch v2.2.0 --depth 1
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

- Optionally, you can copy the binary to a location in your `$PATH`, E.g.:

```bash
sudo cp ./bin/thor /usr/local/bin
```
