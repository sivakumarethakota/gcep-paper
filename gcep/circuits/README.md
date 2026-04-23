# `circuits/` — R_own Groth16 circuit

Reference implementation of the R_own zero-knowledge relation used in the paper's Erasure protocol (Algorithm 3, Phase A).

## What it proves

Given public inputs $(\text{ID}, \text{BID}, \text{CommitHash})$ and a private witness $(\text{PubKey}, \text{Sig}, \text{Msg})$, the circuit asserts:

1. $\text{ID} = \text{MiMC}(\text{PubX}, \text{PubY})$ — the identity commitment binds to the public key.
2. $\text{Msg} = \text{MiMC}(\text{CommitHash}, \text{BID})$ — the message is the canonical commitment-binding.
3. $\text{EdDSA.Verify}(\text{PubKey}, \text{Sig}, \text{Msg})$ — the witness is a valid signature.

A valid proof convinces the verifier that the prover holds the private key that signed commitment `c` at write time, **without revealing the key or public key**.

## Build & run

```bash
# from gcep/circuits
go mod download

# Satisfiability + Prove/Verify roundtrip (fast)
go test -v ./...

# Trusted setup + key export (DEV ONLY)
go run ./cmd/setup -out ./keys
```

Expected output shape:

```
compiling R_own circuit…
compiled in 420ms  (constraints=~25,000)
running Groth16 setup (DEV ONLY — single-party)…
setup complete in ~2s
wrote r1cs.bin        ~3 MB
wrote proving.key     ~15 MB
wrote verifying.key   ~512 bytes
```

## Trusted setup in production

`cmd/setup` runs a **single-party** Groth16 setup. Whoever runs it can forge proofs. For production:

1. Use a multi-party ceremony. `gnark` supports Phase 1 (Powers-of-Tau, universal) and Phase 2 (circuit-specific). Recommended transcript: Aztec's Ignition or Semaphore's — both have hundreds of contributors.
2. Alternatively, switch to a universal-setup SNARK (PLONK via `gnark/backend/plonk`) so only the universal SRS needs a ceremony; circuit-specific setup is deterministic.

## Version notes

This code targets `gnark v0.10.x` and `gnark-crypto v0.12.x`. Two APIs drift across versions — if compilation fails, these are the two places to check:

- **`eddsa.PublicKey.Assign` / `eddsa.Signature.Assign`.** In older gnark releases these took different argument orderings; newer releases use `Assign(curveID, bytes)`. If your version has dropped the helpers, set the struct fields by hand using `PubKey.A.X.Assign(...)` and so on.
- **`hash_native.MIMC_BN254.New()`.** Some gnark-crypto versions expose this as `mimc.NewMiMC(seed)` instead. The in-circuit MiMC (`std/hash/mimc.NewMiMC`) must match the native one byte-for-byte — if public inputs don't verify, this is almost always the cause.

If you hit a version incompatibility, the shortest fix is usually:

```bash
go get github.com/consensys/gnark@v0.10.0
go get github.com/consensys/gnark-crypto@v0.12.1
go mod tidy
```

## How this plugs into chaincode

The chaincode embeds `verifying.key` as a byte constant (or loads it from channel config). On each `Erase` call, it:

1. Parses the incoming proof bytes into a `groth16.Proof`.
2. Reconstructs the public witness from `(c, bid, ctx.ClientID())`.
3. Calls `groth16.Verify(proof, vk, publicWitness)`.

See §3.6 of the paper for the Go chaincode sketch.

## How this plugs into the Caliper workload

`caliper/workloads/erase.js` expects a pre-generated pool of valid proofs in `valid-proofs.bin`. Produce it with:

```bash
go run ./cmd/genproofs -count 1000 -out ./valid-proofs.bin
```

*(The `genproofs` tool is left as a one-day implementation task — it's a wrapper over `groth16.Prove` using `buildValidAssignment` from the test file. Flag for the student: don't commit `proving.key` to Git — it's big and it reveals the setup randomness.)*
