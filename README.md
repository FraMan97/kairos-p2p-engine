# Kairos P2P Engine

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Kubernetes](https://img.shields.io/badge/kubernetes-%23326ce5.svg?style=for-the-badge&logo=kubernetes&logoColor=white)
![Helm](https://img.shields.io/badge/Helm-0F162D?style=for-the-badge&logo=helm&logoColor=white)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?style=for-the-badge&logo=docker&logoColor=white)
![BadgerDB](https://img.shields.io/badge/BadgerDB-FFD700?style=for-the-badge&logo=go&logoColor=black)
![Drand](https://img.shields.io/badge/Drand-141d26?style=for-the-badge&logo=lock&logoColor=white)
![Reed-Solomon](https://img.shields.io/badge/Reed--Solomon-blueviolet?style=for-the-badge)
![Shamir](https://img.shields.io/badge/Shamir_Secret_Sharing-ff69b4?style=for-the-badge)
![License](https://img.shields.io/badge/License-GNU_AGPL_v3-blue?style=for-the-badge&logo=gnu&logoColor=white)

`kairos-p2p-engine` is a high-performance, decentralized peer-to-peer storage engine designed as a cloud-native **Headless** infrastructure. It leverages advanced cryptographic primitives to provide secure, resilient storage with a mathematically enforced **Time-Lock** release mechanism.

The core innovation of Kairos is its integration with the **Drand** beacon network: files remain cryptographically sealed and absolutely inaccessible to anyone—including the storage nodes—until a specific user-defined release time is reached.

> [!WARNING]
> This project is a Proof of Concept (POC) focused on backend infrastructure and is not intended for production environments without further security audits.

---

## Table of Contents

* [System Architecture](#system-architecture)
* [Node Discovery & Auto-Subscription](#node-discovery--auto-subscription)
* [Cryptographic Workflow](#cryptographic-workflow)
* [Key Features](#key-features)
* [CLI Usage Guide](#cli-usage-guide)
* [Installation & Deployment](#installation--deployment)
* [License](#license)

---

## System Architecture

The engine consists of three decoupled components managed via Helm:

### 1. Bootstrap Node (`k-bootstrap`)
The network's central registry.
* Maintains a dynamic list of active storage nodes.
* Hosts **File Manifests** (metadata required for file reconstruction).
* Synchronizes data between bootstrap instances for high availability.
* *Does not handle actual data chunks*.

### 2. Storage Node (`k-node`)
The data persistence layer.
* Provides REST APIs for streaming upload (`/put`), status checks (`/upload/status`), and download (`/get`).
* Stores encrypted data shards in **BadgerDB**, an LSM-tree based key-value store.

### 3. CLI Tool (`k-cli`)
A command-line tool built with Cobra for interacting with the P2P network.

---

## Node Discovery & Auto-Subscription

One of the key features of the engine is the **automatic orchestration of the P2P network**. 

When a **Storage Node** starts, it automatically attempts to register itself with the configured **Bootstrap Servers**:
* The node enters a background loop that signs a subscription request with its **Ed25519** private key.
* It continuously retries registration every 5 seconds until a successful connection is established with a Bootstrap server.
* Once registered, the Bootstrap server records the node's address and public key, making it available to the network for shard distribution.

---

## Cryptographic Workflow

Kairos operates on a zero-trust model where data security is enforced by mathematics rather than node honesty.

### Upload (PUT)
1. **AES Encryption**: Each file block is encrypted using a unique, randomly generated AES-GCM key.
2. **Time-Lock (Drand)**: The AES key is sealed using `tlock`, linking it to a specific future Drand network round corresponding to the desired release time.
3. **Secret Sharing (Shamir)**: The time-locked key is split into multiple fragments using Shamir's Secret Sharing.
4. **Erasure Coding (Reed-Solomon)**: The encrypted data block is fragmented into Data and Parity shards.
5. **Sharded Distribution**: Shards and key fragments are distributed across different nodes. No single node holds enough information to reconstruct the key or the data.

### Download (GET)
1. The node retrieves the File Manifest from the Bootstrap server and fetches the required shards from storage nodes.
2. Data shards are merged (Reed-Solomon) and key fragments are combined (Shamir).
3. The node contacts the Drand network. If the release time has passed, Drand provides the decryption signature needed to unlock the AES key. If the time has not yet been reached, the key remains mathematically locked.

---

## Key Features

* **Zero-Knowledge Architecture**: Nodes store only encrypted fragments and key parts.
* **Cloud-Native Scalability**: Deployed via Helm with K8s StatefulSets for persistent storage.
* **Persistent BadgerDB**: Uses high-throughput LSM local storage for data shards.
* **RAM-Optimized Streaming**: Handles large file transfers through optimized memory buffering and io.Pipe streams.

---

## CLI Usage Guide

The CLI interacts with the network through the Node API (typically on `localhost:8080` via port-forward).

### 1. Upload a file (`put`)
Store a file and set its release date (UTC ISO8601).
```bash
docker run --rm --network host -v $(pwd):/workspace kairos-cli:local \
  put -f /workspace/secret.txt -r "2026-12-31T23:59:59Z"
```

### 2. Download a file (`get`)
Reconstruct and decrypt a file using its unique ID.
```bash
docker run --rm --network host -v $(pwd):/workspace kairos-cli:local \
  get -f <FILE_ID> -o /workspace
```

### 3. Delete a file (`delete`)
Remove a file manifest and all associated shards from the network.
```bash
docker run --rm --network host -v $(pwd):/workspace kairos-cli:local \
  delete -f <FILE_ID>
```

---

## Installation & Deployment

### Local Setup (Kind)
Create a cluster and use the provided deployment scripts:
```bash
kind create cluster --name kairos-vault
./deploy-dev.sh
```
