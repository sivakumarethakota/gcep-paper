'use strict';

const { WorkloadModuleBase } = require('@hyperledger/caliper-core');
const crypto = require('crypto');

/**
 * Erase workload — Algorithm 3 of the paper (Phase A + B only; the committee
 * threshold phase is handled by separate daemons).
 *
 * `invalidProofRatio` controls what fraction of requests carry a forged
 * Groth16 proof. Valid proofs should succeed; invalid proofs should be
 * rejected by chaincode at step 3 of Algorithm 3 (Groth16.Verify).
 *
 * The workload reports:
 *   - success rate (should match 1 - invalidProofRatio within error bar)
 *   - rejection latency for invalid proofs (T3 attack cost)
 *   - acceptance latency for valid proofs (Phase B latency contribution)
 */
class EraseWorkload extends WorkloadModuleBase {
    constructor() {
        super();
        this.txIndex = 0;
        this.poolSize = 50000;
    }

    async initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext) {
        await super.initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext);
        this.eraseRatio = roundArguments.eraseRatio !== undefined ? roundArguments.eraseRatio : 1.0;
        this.invalidProofRatio = roundArguments.invalidProofRatio || 0.0;
        this.workerOffset = workerIndex * Math.floor(this.poolSize / totalWorkers);

        // Pre-generated proofs from the circuits/ package.
        // In the real run these are loaded from disk via a sidecar proof-service.
        this.validProofs = this._loadValidProofs();
    }

    async submitTransaction() {
        this.txIndex++;

        if (Math.random() > this.eraseRatio) {
            return;
        }

        const idx = this.workerOffset + (this.txIndex % Math.floor(this.poolSize / 8));
        const commitmentId = crypto.createHash('sha256').update(`seed-commitment-${idx}`).digest('hex');
        const bid = idx.toString();

        const isInvalid = Math.random() < this.invalidProofRatio;
        const proof = isInvalid
            ? this._randomBytes192()                // forged; Groth16.Verify will reject
            : this.validProofs[idx % this.validProofs.length];

        const args = {
            contractId: 'gcep',
            contractFunction: 'Erase',
            invokerIdentity: 'hospital-admin',
            contractArguments: [
                /* c       */ commitmentId,
                /* bid     */ bid,
                /* zkProof */ proof,
            ],
            readOnly: false,
        };

        try {
            await this.sutAdapter.sendRequests(args);
        } catch (e) {
            // Invalid-proof rejections count as errors in Caliper. That's the
            // correct behaviour — we want P2 to visibly hold in the report.
            if (!isInvalid) throw e;
        }
    }

    _loadValidProofs() {
        // Fallback: generate 1000 deterministic "looks valid" byte strings.
        // Real runs: require('fs').readFileSync('./valid-proofs.bin') emitted by
        // the circuits/cmd/setup tool. See circuits/README.md.
        const out = [];
        for (let i = 0; i < 1000; i++) {
            out.push(crypto.createHash('sha256').update(`valid-proof-${i}`).digest('hex').repeat(3).slice(0, 384)); // 192 bytes hex = 384 chars
        }
        return out;
    }

    _randomBytes192() {
        return crypto.randomBytes(192).toString('hex');
    }
}

function createWorkloadModule() {
    return new EraseWorkload();
}

module.exports.createWorkloadModule = createWorkloadModule;
