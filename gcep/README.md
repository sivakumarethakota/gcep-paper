# GCEP — GDPR-Compliant Erasure Protocol

Reference implementation scaffold for the paper *"GDPR-Compliant Erasure Protocol for Immutable Healthcare Blockchains"*. Provides three paste-ready components:

- **`terraform/`** — AWS infrastructure for the Hyperledger Fabric testbed (4 peers, 3-node RAFT orderer, 5 committee members, 3 IPFS-Cluster nodes, 1 load generator, 1 monitor).
- **`caliper/`** — Hyperledger Caliper 0.6 workload definitions for the three evaluation workloads (W1 steady state, W2 DSR burst, W3 adversarial).
- **`circuits/`** — `gnark` Groth16 circuit for $\mathcal{R}_{\text{own}}$ with setup, prove, and verify commands.

## Quickstart

```bash
# 1. Provision AWS testbed (~$180/week at c6i.2xlarge, $40/week if you size down to t3.medium)
cd terraform
terraform init
terraform apply -var="key_name=YOUR_KEYPAIR" -var="allowed_ssh_cidr=YOUR.IP.0.0/24"

# 2. Generate ZK proving/verifying keys (one-time, ~2 minutes on a laptop)
cd ../circuits
go run ./cmd/setup
# outputs: proving.key, verifying.key, r1cs.bin

# 3. Bring up Fabric via Ansible (not included here — use Fabric's test-network as starting point,
#    point it at the IPs terraform output'd)
ansible-playbook -i $(terraform output -json instance_ips) fabric-up.yml

# 4. Run a benchmark
cd ../caliper
npx caliper launch manager \
  --caliper-networkconfig networkConfig.yaml \
  --caliper-benchconfig benchmarks/w1-steady.yaml \
  --caliper-workspace .
```

## Repo map

```
gcep/
├── README.md                     ← you are here
├── terraform/
│   ├── main.tf                   ← AWS infrastructure (single-file to stay readable)
│   ├── variables.tf              ← tunable inputs
│   └── outputs.tf                ← IPs consumed by Ansible / Caliper
├── caliper/
│   ├── networkConfig.yaml        ← Fabric connection profile
│   ├── benchmarks/
│   │   ├── w1-steady.yaml        ← 70 read / 29 write / 1 erase @ 250–1000 clients, 24h
│   │   ├── w2-dsr-burst.yaml     ← 60 / 20 / 20 @ 500 clients, 30m
│   │   └── w3-adversarial.yaml   ← 50 / 40 / 10 w/ 50% invalid DSRs, 30m
│   └── workloads/
│       ├── write.js              ← Algorithm 1 (Write) client driver
│       ├── read.js               ← Algorithm 2 (Read) client driver
│       └── erase.js              ← Algorithm 3 (Erase) client driver
└── circuits/
    ├── go.mod
    ├── own_circuit.go            ← R_own Groth16 circuit (Poseidon + EdDSA)
    ├── own_circuit_test.go       ← circuit satisfiability + Prove/Verify test
    └── cmd/
        └── setup/main.go         ← Powers-of-Tau style setup (dev only; use real MPC for prod)
```

## What's intentionally **not** here

- The chaincode (Fabric Go smart contract). Track that in a separate repo — the methodology has the sketch in §3.6.
- The committee threshold-BLS daemons. Separate service; `drand/kyber` implementation.
- IPFS-Cluster config. Use the upstream `crdt` template with object-lock pinning.
- Ansible playbooks. Deliberately left as "bring your own" because Fabric provisioning is organisation-specific.

## Costs

AWS on-demand, us-east-1, as of 2026-Q2 pricing:

| Component | Instance type | Count | Hourly | Monthly |
|---|---|---|---|---|
| Fabric peers | `c6i.2xlarge` | 4 | $0.34/h | $980 |
| Orderers | `c6i.large` | 3 | $0.085/h | $184 |
| Committee | `c6i.large` | 5 | $0.085/h | $306 |
| IPFS | `c6i.large` | 3 | $0.085/h | $184 |
| Load gen | `c6i.4xlarge` | 1 | $0.68/h | $490 |
| Monitor | `t3.medium` | 1 | $0.042/h | $30 |
| EBS gp3 | 100 GB × 17 | — | — | $136 |
| **Total** | | | | **~$2,310** |

For the write-up phase, drop all instance types to `t3.medium` (~$400/mo total) and only scale up for the final benchmark runs. The `resource_scale` variable in `variables.tf` does this with one flag.

## Citation

If you build on this, please cite the paper once published and the underlying primitives:

- Ateniese, Magri, Venturi, Andrade. *Redactable Blockchain — or Rewriting History in Bitcoin and Friends.* EuroS&P 2017.
- Gennaro, Jarecki, Krawczyk, Rabin. *Secure Distributed Key Generation for Discrete-Log Based Cryptosystems.* J. Cryptology 2007.
- Groth. *On the Size of Pairing-based Non-interactive Arguments.* EUROCRYPT 2016.
- `gnark` — https://github.com/ConsenSys/gnark
- Hyperledger Fabric 2.5 — https://hyperledger-fabric.readthedocs.io
