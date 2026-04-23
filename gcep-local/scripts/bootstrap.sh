#!/usr/bin/env bash
# bootstrap.sh — one-time setup. Generates Fabric crypto material (TLS certs,
# MSPs) and the channel genesis block. Idempotent: skips if outputs exist.
#
# Requirements: docker (cryptogen + configtxgen are pulled as images).

set -euo pipefail

cd "$(dirname "$0")/.."

CRYPTO_DIR="./crypto-config"
CHANNEL_ARTIFACTS_DIR="./channel-artifacts"
FABRIC_VERSION="${FABRIC_VERSION:-2.5.10}"

if [[ -d "$CRYPTO_DIR" ]] && [[ -d "$CHANNEL_ARTIFACTS_DIR" ]]; then
    echo "[bootstrap] crypto + channel artifacts already exist; nothing to do."
    echo "[bootstrap] remove $CRYPTO_DIR and $CHANNEL_ARTIFACTS_DIR to regenerate."
    exit 0
fi

mkdir -p "$CRYPTO_DIR" "$CHANNEL_ARTIFACTS_DIR" ./configs

# -----------------------------------------------------------------------------
# crypto-config.yaml — orgs, peer counts, user counts
# -----------------------------------------------------------------------------
cat > ./configs/crypto-config.yaml <<'EOF'
OrdererOrgs:
  - Name: Orderer
    Domain: gcep.local
    EnableNodeOUs: true
    Specs:
      - Hostname: orderer0
      - Hostname: orderer1
      - Hostname: orderer2

PeerOrgs:
  - Name: Hospital
    Domain: hospital.gcep.local
    EnableNodeOUs: true
    Template:
      Count: 2
    Users:
      Count: 2

  - Name: Regulator
    Domain: regulator.gcep.local
    EnableNodeOUs: true
    Template:
      Count: 2
    Users:
      Count: 2
EOF

# -----------------------------------------------------------------------------
# configtx.yaml — channel + RAFT consenters + endorsement policy
# -----------------------------------------------------------------------------
cat > ./configs/configtx.yaml <<'EOF'
Organizations:
  - &OrdererOrg
    Name: OrdererOrg
    ID: OrdererMSP
    MSPDir: crypto-config/ordererOrganizations/gcep.local/msp
    Policies:
      Readers:    { Type: Signature, Rule: "OR('OrdererMSP.member')" }
      Writers:    { Type: Signature, Rule: "OR('OrdererMSP.member')" }
      Admins:     { Type: Signature, Rule: "OR('OrdererMSP.admin')" }
    OrdererEndpoints:
      - orderer0.gcep.local:7050
      - orderer1.gcep.local:7050
      - orderer2.gcep.local:7050

  - &Hospital
    Name: HospitalMSP
    ID: HospitalMSP
    MSPDir: crypto-config/peerOrganizations/hospital.gcep.local/msp
    Policies:
      Readers:    { Type: Signature, Rule: "OR('HospitalMSP.admin','HospitalMSP.peer','HospitalMSP.client')" }
      Writers:    { Type: Signature, Rule: "OR('HospitalMSP.admin','HospitalMSP.client')" }
      Admins:     { Type: Signature, Rule: "OR('HospitalMSP.admin')" }
      Endorsement:{ Type: Signature, Rule: "OR('HospitalMSP.peer')" }
    AnchorPeers:
      - Host: peer0-hospital
        Port: 7051

  - &Regulator
    Name: RegulatorMSP
    ID: RegulatorMSP
    MSPDir: crypto-config/peerOrganizations/regulator.gcep.local/msp
    Policies:
      Readers:    { Type: Signature, Rule: "OR('RegulatorMSP.admin','RegulatorMSP.peer','RegulatorMSP.client')" }
      Writers:    { Type: Signature, Rule: "OR('RegulatorMSP.admin','RegulatorMSP.client')" }
      Admins:     { Type: Signature, Rule: "OR('RegulatorMSP.admin')" }
      Endorsement:{ Type: Signature, Rule: "OR('RegulatorMSP.peer')" }
    AnchorPeers:
      - Host: peer0-regulator
        Port: 7051

Capabilities:
  Channel: &ChannelCapabilities
    V2_0: true
  Orderer: &OrdererCapabilities
    V2_0: true
  Application: &ApplicationCapabilities
    V2_5: true

Application: &ApplicationDefaults
  Organizations:
  Policies:
    Readers:    { Type: ImplicitMeta, Rule: "ANY Readers" }
    Writers:    { Type: ImplicitMeta, Rule: "ANY Writers" }
    Admins:     { Type: ImplicitMeta, Rule: "MAJORITY Admins" }
    LifecycleEndorsement: { Type: ImplicitMeta, Rule: "MAJORITY Endorsement" }
    # GCEP endorsement: chaincode-defined, but the channel default is
    # MAJORITY across both orgs.
    Endorsement:          { Type: ImplicitMeta, Rule: "MAJORITY Endorsement" }
  Capabilities:
    <<: *ApplicationCapabilities

Orderer: &OrdererDefaults
  OrdererType: etcdraft
  EtcdRaft:
    Consenters:
      - Host: orderer0.gcep.local
        Port: 7050
        ClientTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer0.gcep.local/tls/server.crt
        ServerTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer0.gcep.local/tls/server.crt
      - Host: orderer1.gcep.local
        Port: 7050
        ClientTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer1.gcep.local/tls/server.crt
        ServerTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer1.gcep.local/tls/server.crt
      - Host: orderer2.gcep.local
        Port: 7050
        ClientTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer2.gcep.local/tls/server.crt
        ServerTLSCert: crypto-config/ordererOrganizations/gcep.local/orderers/orderer2.gcep.local/tls/server.crt
  BatchTimeout: 2s
  BatchSize:
    MaxMessageCount: 500
    AbsoluteMaxBytes: 10 MB
    PreferredMaxBytes: 2 MB
  Organizations:
  Policies:
    Readers:        { Type: ImplicitMeta, Rule: "ANY Readers" }
    Writers:        { Type: ImplicitMeta, Rule: "ANY Writers" }
    Admins:         { Type: ImplicitMeta, Rule: "MAJORITY Admins" }
    BlockValidation:{ Type: ImplicitMeta, Rule: "ANY Writers" }
  Capabilities:
    <<: *OrdererCapabilities

Channel: &ChannelDefaults
  Policies:
    Readers: { Type: ImplicitMeta, Rule: "ANY Readers" }
    Writers: { Type: ImplicitMeta, Rule: "ANY Writers" }
    Admins:  { Type: ImplicitMeta, Rule: "MAJORITY Admins" }
  Capabilities:
    <<: *ChannelCapabilities

Profiles:
  GcepGenesis:
    <<: *ChannelDefaults
    Orderer:
      <<: *OrdererDefaults
      Organizations: [*OrdererOrg]
    Consortiums:
      GcepConsortium:
        Organizations: [*Hospital, *Regulator]

  GcepChannel:
    <<: *ChannelDefaults
    Consortium: GcepConsortium
    Application:
      <<: *ApplicationDefaults
      Organizations: [*Hospital, *Regulator]
EOF

# -----------------------------------------------------------------------------
# Run cryptogen + configtxgen via dockerised tools (no host install needed)
# -----------------------------------------------------------------------------
TOOLS_IMG="hyperledger/fabric-tools:${FABRIC_VERSION}"

echo "[bootstrap] pulling ${TOOLS_IMG}…"
docker pull "${TOOLS_IMG}" >/dev/null

echo "[bootstrap] generating crypto material…"
docker run --rm \
    -v "$PWD":/work -w /work \
    "${TOOLS_IMG}" \
    cryptogen generate --config=./configs/crypto-config.yaml --output="$CRYPTO_DIR"

echo "[bootstrap] generating genesis block…"
docker run --rm \
    -v "$PWD":/work -w /work \
    -e FABRIC_CFG_PATH=/work/configs \
    "${TOOLS_IMG}" \
    configtxgen -profile GcepGenesis -channelID system-channel \
        -outputBlock "$CHANNEL_ARTIFACTS_DIR/genesis.block"

echo "[bootstrap] generating channel tx…"
docker run --rm \
    -v "$PWD":/work -w /work \
    -e FABRIC_CFG_PATH=/work/configs \
    "${TOOLS_IMG}" \
    configtxgen -profile GcepChannel -outputCreateChannelTx \
        "$CHANNEL_ARTIFACTS_DIR/gcepchannel.tx" -channelID gcepchannel

echo
echo "[bootstrap] DONE."
echo "[bootstrap]   crypto material  → $CRYPTO_DIR"
echo "[bootstrap]   channel artifacts → $CHANNEL_ARTIFACTS_DIR"
echo
echo "Next: docker compose -f compose/docker-compose.yaml up -d"
