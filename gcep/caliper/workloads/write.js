'use strict';

const { WorkloadModuleBase } = require('@hyperledger/caliper-core');
const crypto = require('crypto');

/**
 * Write workload — Algorithm 1 of the paper.
 *
 * Each submitTransaction invocation:
 *   1. Generates a synthetic FHIR-shaped PHI payload.
 *   2. Encrypts it off-chain (simulated — real flow goes through IPFS; here
 *      we hash the ciphertext to get a CID placeholder).
 *   3. Derives m = SHA256(cid || bid || policy).
 *   4. Samples randomness r.
 *   5. Computes the chameleon hash c = g^m * y^r in the chaincode side
 *      (we pass m and r; chaincode computes and stores c).
 *   6. Calls chaincode Write(c, bid, policy, sigP, m, r).
 *
 * We deliberately do NOT generate real Schnorr signatures or real chameleon
 * hashes in the load generator — those happen server-side. The workload
 * measures end-to-end chaincode throughput given the cryptographic arguments.
 */
class WriteWorkload extends WorkloadModuleBase {
    constructor() {
        super();
        this.txIndex = 0;
    }

    async initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext) {
        await super.initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext);
        this.writeRatio = roundArguments.writeRatio || 1.0;
        this.workerPrefix = `w${workerIndex}`;
    }

    async submitTransaction() {
        this.txIndex++;

        // Skip on ratio — cheapest way to enforce mix per worker.
        if (Math.random() > this.writeRatio) {
            return;
        }

        const patientId = `${this.workerPrefix}-p${this.txIndex % 10000}`;
        const bid = BigInt(Date.now()) * 1000n + BigInt(this.txIndex);
        const payload = this._syntheticFhir(patientId);

        const blob = crypto.createHash('sha256').update(payload).digest('hex');           // mock CID
        const m = crypto.createHash('sha256').update(`${blob}|${bid}|read-cardio`).digest('hex');
        const r = crypto.randomBytes(32).toString('hex');

        const args = {
            contractId: 'gcep',
            contractFunction: 'Write',
            invokerIdentity: 'hospital-admin',
            contractArguments: [
                /* c       */ '',                    // chaincode computes
                /* bid     */ bid.toString(),
                /* policy  */ 'role:cardiologist AND org:hospital',
                /* sigP    */ crypto.randomBytes(64).toString('hex'),  // placeholder
                /* m       */ m,
                /* r       */ r,
            ],
            readOnly: false,
        };

        await this.sutAdapter.sendRequests(args);
    }

    _syntheticFhir(patientId) {
        // Minimal FHIR R4 Observation bundle — realistic size (~1.5 KB).
        return JSON.stringify({
            resourceType: 'Observation',
            id: `obs-${patientId}-${this.txIndex}`,
            status: 'final',
            code: { coding: [{ system: 'http://loinc.org', code: '8867-4', display: 'Heart rate' }] },
            subject: { reference: `Patient/${patientId}` },
            effectiveDateTime: new Date().toISOString(),
            valueQuantity: { value: 60 + Math.floor(Math.random() * 40), unit: 'beats/minute' },
        });
    }
}

function createWorkloadModule() {
    return new WriteWorkload();
}

module.exports.createWorkloadModule = createWorkloadModule;
