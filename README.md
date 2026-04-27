# Kairos P2P Engine

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Kubernetes](https://img.shields.io/badge/kubernetes-%23326ce5.svg?style=for-the-badge&logo=kubernetes&logoColor=white)
![Helm](https://img.shields.io/badge/Helm-0F162D?style=for-the-badge&logo=helm&logoColor=white)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?style=for-the-badge&logo=docker&logoColor=white)
![BadgerDB](https://img.shields.io/badge/BadgerDB-FFD700?style=for-the-badge&logo=go&logoColor=black)
![Drand](https://img.shields.io/badge/Drand-141d26?style=for-the-badge&logo=lock&logoColor=white)
![Reed-Solomon](https://img.shields.io/badge/Reed--Solomon-blueviolet?style=for-the-badge)
![Shamir](https://img.shields.io/badge/Shamir_Secret_Sharing-ff69b4?style=for-the-badge)

`kairos-p2p-engine` is a high-performance, decentralized peer-to-peer storage engine designed as a cloud-native **Headless** infrastructure. It leverages advanced cryptographic primitives to provide secure, resilient storage with a mathematically enforced **Time-Lock** release mechanism.

The core innovation of Kairos is its integration with the **Drand** beacon network: files remain cryptographically sealed and absolutely inaccessible to anyone—including the storage nodes—until a specific user-defined release time is reached.

> [!WARNING]
> This repository is for portfolio and demonstration purposes only. The source code is copyrighted and no license is granted for its use, modification, or distribution.
> This project is a Proof of Concept (POC) focused on backend infrastructure and is not intended for production environments without further security audits.

---

## Table of Contents

* [System Architecture](#system-architecture)
* [Node Discovery & Health Management](#node-discovery--health-management)
* [Cryptographic Workflow](#cryptographic-workflow)
* [Key Features](#key-features)
* [API & Monitoring](#api--monitoring)
* [CLI Usage Guide](#cli-usage-guide)
* [Installation & Deployment](#installation--deployment)

---

## System Architecture

The engine consists of four decoupled components managed via Helm:

### 1. Bootstrap Node (`k-bootstrap`)
The network's central registry.
* Maintains a dynamic list of active storage nodes and their health status.
* Hosts **File Manifests** required for file reconstruction.
* Manages a **Garbage Collection** worker to remove inactive nodes.
* Synchronizes data between bootstrap instances for high availability.

### 2. Storage Node (`k-node`)
The data persistence layer.
* Provides REST APIs for streaming upload (`/put`), status checks (`/upload/status`), and download (`/get`).
* Stores encrypted data shards in **BadgerDB**.
* Exposes telemetry via the `/metrics` endpoint.

### 3. Explorer (`k-explorer`)
The network monitoring dashboard (Headless).
* Aggregates real-time data from all nodes to provide a global network overview.
* Tracks total secured files, active nodes, and aggregated storage capacity.

### 4. CLI Tool (`k-cli`)
A command-line tool built with Cobra for interacting with the P2P network.

---

## Node Discovery & Health Management

Kairos features a robust **Heartbeat** and **Garbage Collection** mechanism to ensure network reliability:

* **Auto-Subscription & Heartbeat**: When a Storage Node starts, it enters a background loop, automatically registering and re-verifying its presence with **Bootstrap Servers** every 60 seconds.
* **Active Node Registry**: The Bootstrap server records the node's address and public key along with a high-resolution timestamp.
* **Automatic Cleanup**: A dedicated worker on the Bootstrap Node scans the registry every minute. Nodes that fail to send a heartbeat for more than 2 minutes are automatically purged from the active list.

---

## Cryptographic Workflow

Kairos operates on a zero-trust model where data security is enforced by mathematics rather than node honesty.

### Upload (PUT)
1. **AES Encryption**: Each file block is encrypted using a unique, randomly generated AES-GCM key.
2. **Time-Lock (Drand)**: The AES key is sealed using `tlock`, linking it to a specific future Drand round.
3. **Secret Sharing (Shamir)**: The time-locked key is split into multiple fragments using Shamir's Secret Sharing.
4. **Erasure Coding (Reed-Solomon)**: The encrypted block is fragmented into Data and Parity shards.
5. **Sharded Distribution**: Shards and key fragments are distributed across nodes. No single node holds enough information to reconstruct the key or data.

### Download (GET)
1. The node retrieves the File Manifest from the Bootstrap server and fetches required shards from available storage nodes.
2. Data shards are merged and key fragments are combined.
3. The node contacts the Drand network. If the release time has passed, Drand provides the decryption signature needed to unlock the AES key.

---

## Key Features

* **Zero-Knowledge Architecture**: Nodes store only encrypted fragments and key parts.
* **Cloud-Native Scalability**: Deployed via Helm with K8s StatefulSets for persistent storage.
* **Persistent BadgerDB**: Uses high-throughput LSM local storage for data shards.
* **RAM-Optimized Streaming**: Handles large file transfers through optimized memory buffering and io.Pipe streams.

---

## API & Monitoring

The system provides separate OpenAPI documentations for its services:

* **Node API (Port 8085)**: Handles data operations (`/put`, `/get`, `/delete`, `/upload/status`) and local telemetry (`/metrics`).
* **Explorer API (Port 8081)**: Provides a global network state via `/network/overview`.

---

## CLI Usage Guide

The CLI interacts with the network through the Node API on `localhost:8085`.

### 1. Upload a file (`put`)
Store a file and set its release date (UTC ISO8601).
```bash
go run ./cmd/k-cli/main.go put -f ./secret.txt -r "2026-12-31T23:59:59Z"
```

### 2. Download a file (`get`)
Reconstruct and decrypt a file using its unique ID.
```bash
go run ./cmd/k-cli/main.go get -f <FILE_ID> -o ./downloads
```

### 3. Delete a file (`delete`)
Remove a file manifest and all associated shards from the network.
```bash
go run ./cmd/k-cli/main.go delete -f <FILE_ID>
```

---

## Installation & Deployment

### Local Setup (Kind)
Create a cluster and use the provided deployment scripts:
```bash
kind create cluster --name kairos-vault
./deploy-dev.sh
```
The script automatically configures port-forwards:

* **Node API**: http://localhost:8085

* **Network Explorer**: http://localhost:8081/network/overview
