'use strict';

const { WorkloadModuleBase } = require('@hyperledger/caliper-core');
const crypto = require('crypto');

/**
 * Read workload — Algorithm 2 of the paper.
 *
 * We pre-seed the chaincode with a pool of commitment IDs during worker init,
 * then query them in a round-robin fashion. Caliper will capture the Read
 * p50/p95/p99 latency.
 *
 * Note: in the real run the seeding is done by a separate `seed.js` step
 * before Caliper is launched; here we simulate by generating deterministic IDs
 * that the chaincode test harness has pre-created.
 */
class ReadWorkload extends WorkloadModuleBase {
    constructor() {
        super();
        this.txIndex = 0;
        this.poolSize = 50000;
    }

    async initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext) {
        await super.initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext);
        this.readRatio = roundArguments.readRatio || 1.0;
        this.workerOffset = workerIndex * Math.floor(this.poolSize / totalWorkers);
    }

    async submitTransaction() {
        this.txIndex++;

        if (Math.random() > this.readRatio) {
            return;
        }

        const idx = this.workerOffset + (this.txIndex % Math.floor(this.poolSize / 8));
        const commitmentId = this._deterministicCommitmentId(idx);

        const args = {
            contractId: 'gcep',
            contractFunction: 'GetCommitment',
            invokerIdentity: 'hospital-admin',
            contractArguments: [commitmentId],
            readOnly: true,
        };

        await this.sutAdapter.sendRequests(args);
    }

    _deterministicCommitmentId(idx) {
        return crypto.createHash('sha256').update(`seed-commitment-${idx}`).digest('hex');
    }
}

function createWorkloadModule() {
    return new ReadWorkload();
}

module.exports.createWorkloadModule = createWorkloadModule;
