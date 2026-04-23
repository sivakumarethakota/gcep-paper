# GCEP — Local Docker Compose Deployment

A single-machine replacement for the `terraform/` AWS scaffold. Brings up the entire stack — 4 Fabric peers, 3-node RAFT orderer, 5 committee members, 3 IPFS-Cluster nodes, Prometheus + Grafana — as Docker containers on your PC.

## Sizing & profiles

The compose file uses **Docker Compose profiles** so you can dial the resource budget up or down without editing files.

| Profile | What's included | RAM (idle) | RAM (under load) | Best for |
|---|---|---|---|---|
| `tiny` | 1 peer, 1 orderer, 1 committee, 1 IPFS, monitoring | ~2 GB | ~4 GB | Chaincode + circuit dev on a laptop |
| `small` | 2 peers, 1 orderer, 3 committee, 1 IPFS, monitoring | ~4 GB | ~8 GB | Functional E2E testing, low-load Caliper |
| `full` | 4 peers, 3 orderers, 5 committee, 3 IPFS, monitoring | ~8 GB | ~16 GB | Paper-grade local benchmarks |

Pick by setting `COMPOSE_PROFILES`:

```bash
export COMPOSE_PROFILES=small        # default in .env
docker compose up -d
```

## What's different from the AWS path

| Concern | AWS | Local |
|---|---|---|
| Inter-node latency | ~0.5 ms (intra-AZ) | ~0.05 ms (loopback) |
| TPS ceiling | Bounded by Fabric + network | Bounded by your CPU + container scheduler |
| Multi-AZ failure tests | Realistic | Simulated only — kill containers, not regions |
| Cost | $400–$2300/month | Electricity |
| Setup time | 30 min (Terraform + Ansible) | 5 min (`docker compose up`) |

The Caliper workloads, the gnark circuit, and the chaincode are **identical**. Only the network config switches: `caliper/networkConfig.local.yaml` points at `localhost:7051..7054` instead of remote IPs.

## Honest reporting in your paper

When you write this up, report:
- Single-host configuration with hardware spec (CPU model, core count, RAM, kernel).
- Container limits (we pin RAM via Compose's `mem_limit`).
- Localhost-network caveat — TPS will be 1.5–3× higher than a multi-host deployment.

For the **final** numbers in your paper, run a 3-day AWS sprint with the `terraform/` scaffold and report both — local development numbers and cloud production numbers. Reviewers respect this kind of disclosure.

## Quickstart

```bash
# 0. Prerequisites
docker --version           # 24.0+
docker compose version     # v2.20+
# Linux: ensure your user is in the docker group
# macOS/Windows: Docker Desktop, raise RAM limit to 8+ GB in settings

# 1. One-time bootstrap: generate Fabric crypto material + genesis block
./scripts/bootstrap.sh

# 2. Bring up the stack
export COMPOSE_PROFILES=small
docker compose -f compose/docker-compose.yaml up -d

# 3. Wait for everything to settle
./scripts/wait-ready.sh

# 4. Deploy the GCEP chaincode (run from your chaincode repo)
./scripts/deploy-chaincode.sh ../chaincode

# 5. Smoke test
./scripts/smoke-test.sh

# 6. Run a benchmark
cd ../caliper
npx caliper launch manager \
    --caliper-networkconfig networkConfig.local.yaml \
    --caliper-benchconfig benchmarks/w1-steady.yaml \
    --caliper-workspace .

# 7. Tear down (preserves volumes)
docker compose -f compose/docker-compose.yaml down

# 8. Tear down + wipe everything
docker compose -f compose/docker-compose.yaml down -v
```

## Port map (host → container)

Everything is exposed on `127.0.0.1` by default. Override `BIND_HOST` in `.env` to expose on the LAN.

| Service | Host port | Container port |
|---|---|---|
| peer0.hospital | 7051 | 7051 |
| peer0.hospital chaincode | 7052 | 7052 |
| peer1.hospital | 8051 | 7051 |
| peer0.regulator | 9051 | 7051 |
| peer1.regulator | 10051 | 7051 |
| orderer0 | 7050 | 7050 |
| orderer1 | 7150 | 7050 |
| orderer2 | 7250 | 7050 |
| ipfs0 swarm/api | 4001 / 5001 | 4001 / 5001 |
| Prometheus | 9090 | 9090 |
| Grafana | 3000 | 3000 |
| node-exporter | 9100 | 9100 |
| cAdvisor (per-container CPU/RAM) | 8080 | 8080 |

## Troubleshooting

**"address already in use" on 7050/7051.** Another Fabric instance is running. `docker ps` to find it.

**Out of memory mid-benchmark.** Drop `COMPOSE_PROFILES` from `full` to `small`. Lower the Caliper `transactionLoad`. On macOS, raise Docker Desktop's memory ceiling in Preferences → Resources.

**Caliper "MVCC_READ_CONFLICT" errors at high load.** Expected when the workload writes to the same keys faster than block commits. Spread keys across more workers, or add `await new Promise(r => setTimeout(r, 5))` between writes in the workload module.

**Prometheus scrape returns nothing.** The `monitoring` profile must be active. Check with `docker compose ps`. Grafana auth is `admin/admin` on first login.

**Containers crash-loop on Apple Silicon.** Hyperledger publishes `linux/arm64` images for Fabric 2.5.5+, but a few sidecars don't. Add `platform: linux/amd64` to those services and accept the Rosetta penalty.
