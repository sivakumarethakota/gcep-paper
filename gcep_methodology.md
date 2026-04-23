# GDPR-Compliant Erasure Protocol (GCEP) for Immutable Healthcare Blockchains

*Methodology section — paste-ready for a paper draft. Convert to LaTeX by replacing Markdown headings with `\section`/`\subsection`, fenced code blocks with `lstlisting`, and keeping inline math `$...$` as-is.*

---

## 3.1 Preliminaries and Notation

Let $\lambda$ be the security parameter. We work over a cyclic group $\mathbb{G}$ of prime order $q$ with generator $g$, in which the discrete-logarithm problem is $(\lambda,\varepsilon)$-hard (we instantiate on BN254 for compatibility with Groth16). Let:

- $H_{\text{sha}}: \{0,1\}^* \to \{0,1\}^{256}$ — SHA-256, used only outside the chameleon construction;
- $\text{AES-GCM}_k(\cdot)$ — authenticated symmetric encryption with key $k$;
- $\pi_{\text{zk}}$ — a Groth16 proof over the BN254 pairing curve;
- $\mathcal{C}$ — the erasure committee, a set of $n$ regulators / notaries with threshold $t$ (we use $t=3, n=5$);
- $P$ — a patient (data subject); $H$ — a hospital; $R$ — a regulator.

Table 1 lists all notation used in the protocol.

| Symbol | Meaning |
|---|---|
| $(x, y)$ | Chameleon-hash trapdoor $x \in \mathbb{Z}_q$ and public key $y = g^x$ |
| $(x_i)_{i=1}^n$ | Shamir shares of $x$ with reconstruction threshold $t$ |
| $CH(m, r)$ | Chameleon hash of message $m$ with randomness $r$ |
| $(sk_P, pk_P)$ | Patient key pair; $id_P = H_{\text{sha}}(pk_P)$ |
| $\text{blob}$ | Off-chain encrypted PHI payload |
| $c$ | On-chain commitment (chameleon-hashed pointer to $\text{blob}$) |
| $\sigma_\mathcal{C}$ | Threshold signature by committee $\mathcal{C}$ |

---

## 3.2 System Model

The system has four roles.

**Patients ($P$)** own health data and hold identity key pairs $(sk_P, pk_P)$. A patient can issue a data-subject request (DSR) to erase, rectify, or export their records.

**Hospitals ($H$)** operate Hyperledger Fabric peers, endorse transactions, and run an off-chain encrypted object store (IPFS-Cluster with S3-compatible object-lock). PHI never leaves the hospital's encrypted store in plaintext.

**Regulators / committee ($\mathcal{C}$)** hold threshold shares of the chameleon-hash trapdoor. They can collectively rewrite an on-chain commitment *iff* a valid erasure request is presented, and only at the commitment the patient has authority over. No subset smaller than $t$ can do so.

**The ledger ($\mathcal{L}$)** is a Hyperledger Fabric channel with RAFT ordering. It stores chameleon-hashed commitments, access-policy transactions, and audit events — **never plaintext PHI**.

Two design invariants follow:

- **I1 — PHI separation.** On-chain data contains only commitments and metadata; no plaintext or ciphertext of PHI appears on $\mathcal{L}$.
- **I2 — Block-structure preservation.** Erasure rewrites the pre-image of a commitment, not the commitment itself. Merkle roots, block hashes, and downstream chain continuity are untouched.

## 3.3 Threat Model

We consider a probabilistic polynomial-time adversary $\mathcal{A}$ with the following capabilities.

- $\mathcal{A}$ may corrupt up to $t-1$ committee members and learn their trapdoor shares.
- $\mathcal{A}$ may corrupt any subset of hospitals, including their Fabric peers and object stores.
- $\mathcal{A}$ may observe all ledger traffic and all off-chain network traffic.
- $\mathcal{A}$ may compromise any number of patient devices *other than* the target patient.

$\mathcal{A}$ may **not** break the discrete-log assumption on BN254, the AEAD security of AES-GCM, the soundness of Groth16, or the collision-resistance of SHA-256 with non-negligible probability.

We seek four properties:

- **P1 — Right to erasure.** After a successful erasure, no PPT adversary can recover the erased PHI from any combination of on-chain and retained off-chain data, except with negligible probability.
- **P2 — No unauthorised rewrite.** Only an erasure that carries a valid patient ZK proof *and* a threshold committee signature produces a chain state transition.
- **P3 — Audit completeness.** Every erasure produces an on-chain event log entry that is itself immutable under the same chameleon trapdoor.
- **P4 — Downstream continuity.** Blocks committed after an erasure remain valid; no fork, no re-validation, no replay.

---

## 3.4 Cryptographic Primitives

### 3.4.1 Chameleon Hash (Krawczyk–Rabin / Ateniese form)

A chameleon hash is a keyed collision-resistant hash that becomes collision-*finding* with a trapdoor. We instantiate the DL-based construction:

**Setup.** Sample $x \overset{\$}{\leftarrow} \mathbb{Z}_q$; set $y \leftarrow g^x$. Publish $(\mathbb{G}, q, g, y)$; keep $x$ as the trapdoor.

**Hash.** For message $m \in \mathbb{Z}_q$ and randomness $r \in \mathbb{Z}_q$:

$$CH(m, r) \;=\; g^m \cdot y^r \bmod p$$

**Verification.** Recompute and compare.

**Collision (trapdoor-only).** Given a valid $(m, r)$ and a target message $m'$, the trapdoor holder computes

$$r' \;=\; r + x^{-1}(m - m') \bmod q$$

and $CH(m', r') = CH(m, r)$ holds by construction.

Without $x$, finding $(m', r') \neq (m, r)$ with $CH(m', r') = CH(m, r)$ reduces to discrete log in $\mathbb{G}$.

### 3.4.2 Threshold Chameleon Hash

We split the trapdoor $x$ using Shamir secret sharing over $\mathbb{Z}_q$:

1. Sample a degree-$(t-1)$ polynomial $f(Z) = x + a_1 Z + \dots + a_{t-1} Z^{t-1}$.
2. Distribute share $x_i = f(i)$ to committee member $\mathcal{C}_i$ for $i = 1, \dots, n$.

To compute a collision, any $t$ members engage in the following protocol (executed over TLS-authenticated channels between committee members):

1. Each participant $\mathcal{C}_i$ computes the partial $r'_i = r + \lambda_i \cdot x_i^{-1}(m - m') \bmod q$, where $\lambda_i$ is their Lagrange coefficient for the active subset.
2. Participants broadcast $r'_i$.
3. The collision is reconstructed as $r' = \sum_{i \in S} r'_i \bmod q$ for the active set $S$ of size $t$.

This avoids reconstructing $x$ in the clear on any single machine.

### 3.4.3 Zero-Knowledge Proof of Ownership

We define a Groth16 circuit $\mathcal{R}_{\text{own}}$ with:

- **Public inputs:** $id_P$, block identifier $bid$, target commitment $c$.
- **Private witness:** $sk_P$, $pk_P$, and a Schnorr signature $\sigma_{P \to c}$ that binds $sk_P$ to the commitment $c$ at write time.

$\mathcal{R}_{\text{own}}$ verifies:

- $id_P = H_{\text{sha}}(pk_P)$;
- $pk_P = g^{sk_P}$;
- $\sigma_{P \to c}$ is a valid Schnorr signature under $pk_P$ over $(c, bid)$.

A valid proof $\pi_{\text{zk}}$ convinces any verifier that the prover authored the commitment $c$ without revealing $sk_P$ or $pk_P$. Proof size is 192 bytes; verification runs in $\sim 3$ ms.

### 3.4.4 Off-Chain Encrypted Storage

PHI payloads are stored in an IPFS-Cluster backed by S3-compatible object-lock buckets. Each payload $D$ is serialised in HL7 FHIR R4, encrypted as $\text{blob} = \text{AES-GCM}_{k_P}(D)$ where $k_P$ is derived from a per-patient CP-ABE key, and addressed by its content identifier $cid = H_{\text{sha}}(\text{blob})$. The on-chain commitment is

$$c \;=\; CH(m, r), \qquad m = H_{\text{sha}}(cid \;\|\; bid \;\|\; \text{policy}).$$

Erasure consists of two coupled actions: deleting $\text{blob}$ from the object store and rewriting the pre-image of $c$ on-chain so that the commitment no longer points to recoverable data.

---

## 3.5 GDPR-Compliant Erasure Protocol (GCEP)

GCEP has three subprotocols: Write, Read, and Erase.

### 3.5.1 Setup

Committee $\mathcal{C}$ runs a DKG protocol (Gennaro et al. 2007) to produce $(y, \{x_i\}_{i=1}^n)$ without any single dealer ever holding $x$. The public key $y$ is published in the Fabric channel's genesis block. The Groth16 common reference string for $\mathcal{R}_{\text{own}}$ is generated via a multi-party Powers-of-Tau ceremony and similarly published.

### 3.5.2 Write Protocol

```text
Algorithm 1 — Write(P, H, D, policy)
Input:  Patient P, hospital H, PHI record D, access policy policy
Output: On-chain commitment c, off-chain blob identifier cid

1.  k_P   ← CP-ABE.KeyDerive(P, policy)
2.  blob  ← AES-GCM_{k_P}(D)
3.  cid   ← H_sha(blob)
4.  STORE blob at cid in IPFS-Cluster with object-lock
5.  bid   ← ledger.nextBlockId()
6.  m     ← H_sha(cid ‖ bid ‖ policy)
7.  r     ←$ Z_q                            # uniform randomness
8.  c     ← g^m · y^r  mod p                # chameleon hash
9.  σ_P   ← Schnorr.Sign(sk_P, c ‖ bid)     # binds patient to commitment
10. tx    ← (c, bid, policy, σ_P, m, r)
11. SUBMIT tx to Fabric channel via chaincode Write()
12. RETURN (c, cid)
```

Note that $(m, r)$ are stored **on-chain** alongside $c$; the trapdoor holder needs them later to compute a collision. This is safe because $m$ is itself a hash of $(cid, bid, \text{policy})$, none of which is PHI.

### 3.5.3 Read Protocol

```text
Algorithm 2 — Read(requester Q, commitment c)
Input:  Requester Q with role attributes attrs_Q
Output: Plaintext D or ⊥

1.  (c, bid, policy, σ_P, m, r) ← ledger.getCommitment(c)
2.  IF NOT policy.evaluate(attrs_Q): RETURN ⊥
3.  cid ← decode(m)                         # m = H_sha(cid ‖ bid ‖ policy)
4.  blob ← IPFS.fetch(cid)
5.  IF blob = ⊥: RETURN ⊥                   # erasure already happened
6.  k_Q  ← CP-ABE.KeyDerive(Q, policy)
7.  D    ← AES-GCM.Decrypt(k_Q, blob)
8.  ledger.emitEvent(Access, Q, c, now())
9.  RETURN D
```

### 3.5.4 Erasure Protocol

The core contribution. Erasure must simultaneously destroy the off-chain PHI and neutralise the on-chain pre-image, while preserving block structure.

```text
Algorithm 3 — Erase(P, c)
Input:  Patient P issuing DSR on commitment c
Output: Success flag

Phase A — Patient-side proof
1.  π_zk ← Groth16.Prove(R_own;
             public=(id_P, bid, c);
             witness=(sk_P, pk_P, σ_{P→c}))
2.  SUBMIT (EraseRequest, c, bid, π_zk) to chaincode Erase()

Phase B — Chaincode validation
3.  IF NOT Groth16.Verify(R_own, (id_P, bid, c), π_zk): ABORT
4.  IF ledger.alreadyErased(c): ABORT
5.  ledger.setStatus(c, PENDING_ERASURE)
6.  ledger.emitEvent(EraseRequested, id_P, c, now())

Phase C — Committee threshold rewrite
7.  Committee members C_1, …, C_n observe EraseRequested event
8.  A quorum S ⊆ C of size t opens a threshold session
9.  Target pre-image m' ← H_sha("∅ERASED" ‖ bid ‖ now())
10. For each C_i ∈ S:
       r'_i ← r + λ_i · x_i^{-1} · (m - m')  mod q
11. r'   ← Σ_{i ∈ S} r'_i  mod q
12. σ_C  ← ThresholdBLS.Sign(S, (c, m', r'))

Phase D — Ledger update and object deletion
13. SUBMIT (EraseCommit, c, m', r', σ_C) to chaincode FinaliseErase()
14. IF NOT ThresholdBLS.Verify(σ_C): ABORT
15. ASSERT g^{m'} · y^{r'} mod p == c           # collision holds
16. ledger.setPreimage(c, m', r')
17. ledger.setStatus(c, ERASED)
18. IPFS.delete(cid) on all replicas                # off-chain destruction
19. ledger.emitEvent(Erased, id_P, c, now())
20. RETURN success
```

**Key property.** After step 16 the on-chain commitment $c$ is unchanged — same bytes, same Merkle inclusion, same block hash. Its pre-image has moved from $(m, r)$ pointing at $cid$ to $(m', r')$ pointing at $\varnothing$. Block-structure preservation (invariant I2) is achieved by construction.

---

## 3.6 Smart Contract Design (Hyperledger Fabric Chaincode)

We sketch the key transactions; a full Go implementation is provided in the public artefact (Section 3.8.1).

```go
type Commitment struct {
    C          []byte   // chameleon hash (as bytes)
    BID        uint64
    Policy     Policy
    M          []byte   // pre-image message
    R          []byte   // pre-image randomness
    SigPatient []byte
    Status     string   // "ACTIVE" | "PENDING_ERASURE" | "ERASED"
}

func (s *SmartContract) Write(
    ctx, c, bid, policy, sigP, m, r []byte,
) error {
    // anchor commitment; M and R stored to enable future trapdoor rewrite
}

func (s *SmartContract) Erase(
    ctx, c, bid, zkProof []byte,
) error {
    require(Groth16Verify(R_own,
        publicInputs(c, bid, ctx.ClientID()), zkProof))
    commit := getCommitment(c)
    require(commit.Status == "ACTIVE")
    commit.Status = "PENDING_ERASURE"
    putCommitment(commit)
    emitEvent("EraseRequested", c, bid, now())
    return nil
}

func (s *SmartContract) FinaliseErase(
    ctx, c, mPrime, rPrime, sigC []byte,
) error {
    commit := getCommitment(c)
    require(commit.Status == "PENDING_ERASURE")
    require(ThresholdBLSVerify(sigC, (c, mPrime, rPrime)))
    require(ChameleonCheck(c, mPrime, rPrime))
    commit.M, commit.R = mPrime, rPrime
    commit.Status = "ERASED"
    putCommitment(commit)
    emitEvent("Erased", c, now())
    return nil
}
```

Endorsement policy: `AND(OrgHospital.peer, MAJORITY OrgRegulator)`. This ensures no single hospital can forge erasure, and no single regulator can bypass hospital endorsement.

---

## 3.7 Security Analysis

We sketch proofs for P1–P4.

**Lemma 1 (P1 — right to erasure).** *After a successful run of Algorithm 3, the probability that a PPT adversary $\mathcal{A}$ recovers the pre-erasure PHI $D$ is $\leq \varepsilon_{\text{AES}}(\lambda) + \varepsilon_{\text{ABE}}(\lambda)$ where $\varepsilon_{\text{AES}}$ is AES-GCM's IND-CCA advantage and $\varepsilon_{\text{ABE}}$ is the CP-ABE key-secrecy advantage.*

*Proof sketch.* After step 18, $\text{blob}$ is deleted from every replica and the IPFS DHT entry is tombstoned. The on-chain pre-image $(m', r')$ encodes only $H_{\text{sha}}(\text{``}\varnothing\text{ERASED''} \| bid \| t_{\text{erase}})$, which is independent of $D$. Any residual cached ciphertext held by $\mathcal{A}$ is useless absent $k_P$; CP-ABE key secrecy reduces to bilinear Diffie–Hellman. $\square$

**Lemma 2 (P2 — no unauthorised rewrite).** *The probability that $\mathcal{A}$ controlling up to $t-1$ committee shares and arbitrary hospitals produces a state transition that sets $\text{Status}(c) = \text{ERASED}$ is $\leq \varepsilon_{\text{DL}}(\lambda) + \varepsilon_{\text{Groth}}(\lambda)$.*

*Proof sketch.* Chaincode `FinaliseErase` requires $\sigma_\mathcal{C}$ verifiable under the committee's aggregate public key. Forging $\sigma_\mathcal{C}$ from fewer than $t$ shares reduces to DL on BN254; Groth16 soundness precludes forging $\pi_{\text{zk}}$ without $sk_P$. $\square$

**Lemma 3 (P3 — audit completeness).** *Every erasure emits an `Erased` event whose entry in the channel's event store is itself committed via the standard Fabric block path and thus subject to the same chameleon commitments. Erasure of an erasure record would require its own DSR and committee quorum.* $\square$

**Lemma 4 (P4 — downstream continuity).** *Because only $(m, r) \to (m', r')$ changes and $CH(m, r) = CH(m', r')$ by the collision property of Section 3.4.1, the commitment $c$ stored in block $bid$ is byte-identical before and after erasure. Merkle roots, block hashes, and all subsequent blocks remain valid.* $\square$

---

## 3.8 Implementation Plan

### 3.8.1 Software Stack

| Component | Technology | Version |
|---|---|---|
| Blockchain | Hyperledger Fabric | 2.5.5 |
| Consensus | RAFT | built-in |
| Chaincode | Go | 1.22 |
| Chameleon hash | Custom Go pkg on `cloudflare/bn256` | — |
| ZK proofs | `gnark` Groth16 (BN254) | 0.9 |
| Threshold BLS | `drand/kyber` | latest |
| Off-chain store | IPFS-Cluster + MinIO object-lock | 1.0 / RELEASE.2025 |
| PHI format | HL7 FHIR R4 | R4 |
| Clinician auth | SMART-on-FHIR / Keycloak | 24.x |
| Orchestration | Docker Compose + Kubernetes | — |
| Benchmarking | Hyperledger Caliper | 0.6 |
| Telemetry | Prometheus + Grafana | latest |

### 3.8.2 Testbed Hardware

- **Fabric peers**: 4 × AWS `c6i.2xlarge` (8 vCPU, 16 GB RAM) across two AZs.
- **Orderer**: 3-node RAFT cluster on `c6i.large`.
- **Committee members**: 5 × `c6i.large`, geographically distributed.
- **Client load generators**: `c6i.4xlarge`.
- **IPFS-Cluster**: 3 storage nodes with EBS gp3 volumes.

This replaces the paper-under-review's Celeron N4020 — every reported number will be reproducible and honest.

---

## 3.9 Experiment Plan

### 3.9.1 Research Questions

- **RQ1.** What is the latency overhead of a chameleon-based write versus a plain SHA-256 write?
- **RQ2.** What is the end-to-end erasure latency (DSR submit $\to$ `Erased` event)?
- **RQ3.** What is the throughput impact under sustained mixed workload (read/write/erase)?
- **RQ4.** How does the system scale with committee size $n$ and threshold $t$?
- **RQ5.** How does GCEP compare to (a) immutable baselines and (b) prior redactable-chain proposals?
- **RQ6.** Under adversarial conditions (up to $t-1$ corrupted committee members; poisoned DSR requests), does the system maintain P1–P4?

### 3.9.2 Workloads

We derive a realistic hospital workload from MIMIC-IV timestamps plus a synthetic DSR rate aligned with published GDPR DSR statistics (≈0.02 DSR per patient-year for health data):

| Workload | Read : Write : Erase | Duration | Concurrent clients |
|---|---|---|---|
| W1 — steady state | 70 : 29 : 1 | 24 h | 250, 500, 1000 |
| W2 — DSR burst | 60 : 20 : 20 | 30 min | 500 |
| W3 — adversarial | 50 : 40 : 10 (50% invalid DSRs) | 30 min | 500 |

### 3.9.3 Baselines

| Baseline | Description |
|---|---|
| **B1 — SHA-256 immutable** | Standard Fabric with SHA-256; erasure impossible (illustrates GDPR non-compliance) |
| **B2 — Off-chain deletion only** | Delete blob, leave dangling on-chain hash (EDPB 02/2025 minimum) |
| **B3 — Ateniese et al. (single-trustee)** | Chameleon hash with single trapdoor holder |
| **B4 — Deuber et al. (2019) redactable chain** | Chameleon-on-block with PoW voting |
| **GCEP (ours)** | Threshold chameleon + ZK + event log |

### 3.9.4 Metrics

- **Latency:** write p50/p95/p99; read p50/p95/p99; erasure end-to-end p50/p95/p99.
- **Throughput:** sustained TPS under each workload; degradation vs. B1.
- **Storage:** per-commitment on-chain footprint (bytes); per-erasure delta.
- **Committee cost:** threshold-session wall clock; per-member CPU-ms and network bytes.
- **Compliance:** fraction of DSRs resolved within GDPR's 30-day deadline (target: 100%).
- **Security:** per-attack success rate (see 3.9.6).

### 3.9.5 Ablations

- A1. Trapdoor holder: single vs. threshold ($t=2,3,5$).
- A2. ZK proof system: Groth16 vs. PLONK vs. Bulletproofs (proof size, verify time).
- A3. Off-chain store: IPFS-Cluster vs. Ceph vs. S3 with object-lock.
- A4. Commitment batching: one-per-tx vs. batched Merkle of 100 commitments rewritten together.
- A5. Endorsement policy: `OR(Hospital, Regulator)` vs. `AND(Hospital, MAJORITY Regulator)`.

### 3.9.6 Adversarial Simulation

We implement concrete attacks and report success rate.

| Attack | Threat | Expected result |
|---|---|---|
| T1 — Committee collusion ($t-1$ corrupt) | Unauthorised rewrite | Fails: threshold not met |
| T2 — Replay of stale DSR | Erase a different commitment | Fails: $bid$ bound in circuit |
| T3 — Forged ZK proof | Erase without owning the data | Fails: Groth16 soundness |
| T4 — Hospital resurrection | Restore erased blob from backup | Fails: key rotated, ciphertext undecryptable |
| T5 — Chain fork after erasure | Reintroduce erased commitment on forked chain | Fails: Fabric orderer consensus + endorsement |
| T6 — DoS on committee | Block erasure indefinitely | Partial: measured time-to-quorum under load |

Implementation with `art`, custom Go fuzzers for T2–T3, and Chaos-Mesh for T6.

### 3.9.7 Statistical Reporting

Every reported number is mean $\pm$ 95% CI over 5 independent runs. Paired comparisons against each baseline use the Wilcoxon signed-rank test with Bonferroni correction. We release a pre-registered protocol and a reproducibility package (Docker Compose, dataset manifests, seed files, Makefile) on Zenodo with a DOI.

---

## 3.10 Compliance Mapping

| Regulation | Clause | GCEP mechanism |
|---|---|---|
| GDPR Art. 17 | Right to erasure | Algorithm 3 |
| GDPR Art. 16 | Right to rectification | Variant of Alg. 3 with $m' = H_{\text{sha}}(cid_{\text{new}} \| \cdots)$ |
| GDPR Art. 25 | Data protection by design | Invariants I1, I2; off-chain PHI by default |
| GDPR Art. 32 | Security of processing | AES-GCM, threshold trapdoor, ZK proofs |
| EDPB Guidelines 02/2025 | PII off-chain + commitments on-chain | §3.4.4 and §3.5.2 |
| HIPAA §164.312(a)(2)(iv) | Encryption of ePHI | AES-GCM + CP-ABE |
| HIPAA §164.528 | Accounting of disclosures | Access / Erase event log |
| India DPDP Act §12 | Right to erasure | Algorithm 3 |
| India DPDP Act §8(7) | Data retention limits | Automated erasure on retention-policy expiry (variant of Alg. 3) |

A compliance officer can tick every row by inspecting the public chaincode and the audit event stream; no side agreement needed.

---

## 3.11 Limitations and Scope

- GCEP assumes the trusted setup of both the chameleon key pair and the Groth16 CRS. We mitigate via multi-party ceremonies but do not eliminate.
- Erasure of on-chain *metadata* (e.g., policy expressions naming third parties) requires the same protocol; we show this but do not benchmark it separately.
- Post-quantum: BN254 and Groth16 are not post-quantum secure. A PQ port using Falcon signatures, SNARKs over lattice commitments (Lasso/Jolt lineage), and LWE-based chameleon hashes is future work.
- GCEP does not address erasure of data already exported by authorised readers before the DSR. This is a fundamental limit of any ledger-based approach and must be handled by downstream processor agreements (GDPR Art. 28).

---

## 3.12 Summary of Contributions

1. A threshold chameleon-hash construction that lets a regulator committee — not any single party — rewrite on-chain commitments, preserving block structure (I2).
2. A Groth16 circuit $\mathcal{R}_{\text{own}}$ that lets a patient prove authorship of a commitment without revealing identity material, enabling pseudonymous DSRs.
3. The GCEP protocol tying off-chain encrypted PHI deletion to on-chain pre-image rewriting in a single auditable workflow.
4. A reproducible Fabric implementation with chaincode, Caliper workloads, adversarial simulator, and compliance mapping to GDPR, HIPAA, and India's DPDP Act.
5. Security proofs of P1–P4 under standard cryptographic assumptions.

---

## Next steps for the paper

- **Results section:** fill in numbers from Section 3.9 once the testbed runs. Tables are pre-structured so you can paste in directly.
- **Related work:** position against Ateniese et al. 2017, Deuber et al. 2019, Puddu et al. 2017 (chain-edit), Derler et al. (policy-based chameleon), and the EDPB 2025 guidance.
- **Discussion:** argue why GCEP's threshold design is the minimum trust assumption compatible with Art. 17 obligations — a single trustee fails the independence requirement EDPB signals in §7.2 of the 2025 guidelines.

*Ready to adapt for IEEE IoT-J, TDSC, ACM TOPS, or FC if you want the section re-shaped to a specific venue's style.*
