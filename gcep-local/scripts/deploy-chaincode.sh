#!/usr/bin/env bash
# deploy-chaincode.sh — Fabric 2.x lifecycle deploy for the GCEP chaincode.
#
# Usage:  ./deploy-chaincode.sh <path-to-chaincode-source>
# Example: ./deploy-chaincode.sh ../chaincode

set -euo pipefail

CC_SRC="${1:-../chaincode}"
CC_NAME="${CC_NAME:-gcep}"
CC_VERSION="${CC_VERSION:-1.0}"
CC_SEQUENCE="${CC_SEQUENCE:-1}"
CC_LANG="${CC_LANG:-golang}"
CHANNEL="${CHANNEL:-gcepchannel}"
ORDERER="${ORDERER:-orderer0.gcep.local:7050}"
ORDERER_TLS_CA="/etc/hyperledger/fabric/crypto-config/ordererOrganizations/gcep.local/orderers/orderer0.gcep.local/msp/tlscacerts/tlsca.gcep.local-cert.pem"

if [[ ! -d "$CC_SRC" ]]; then
    echo "ERROR: chaincode source directory '$CC_SRC' does not exist." >&2
    echo "Pass the path to your chaincode repo as the first argument." >&2
    exit 1
fi

# Copy chaincode into both peers' work area
echo "[deploy] copying chaincode into peer containers…"
for peer in peer0.hospital.gcep.local peer0.regulator.gcep.local; do
    if docker ps --format '{{.Names}}' | grep -q "^${peer}\$"; then
        docker exec "${peer}" rm -rf /opt/gopath/src/${CC_NAME} 2>/dev/null || true
        docker cp "$CC_SRC" "${peer}:/opt/gopath/src/${CC_NAME}"
    fi
done

# Endorsement: at least one peer per org
ENDORSEMENT="AND('HospitalMSP.peer','RegulatorMSP.peer')"

peer_exec() {
    local peer="$1" mspid="$2" mspuser="$3" tlsca="$4"
    shift 4
    docker exec \
        -e CORE_PEER_LOCALMSPID="$mspid" \
        -e CORE_PEER_MSPCONFIGPATH="/etc/hyperledger/fabric/crypto-config/peerOrganizations/${peer#peer*.}/users/${mspuser}/msp" \
        -e CORE_PEER_TLS_ENABLED=true \
        -e CORE_PEER_TLS_ROOTCERT_FILE="/etc/hyperledger/fabric/crypto-config/peerOrganizations/${peer#peer*.}/peers/${peer}/tls/ca.crt" \
        "$peer" "$@"
}

echo "[deploy] packaging chaincode…"
peer_exec peer0.hospital.gcep.local HospitalMSP "Admin@hospital.gcep.local" \
    "/etc/hyperledger/fabric/crypto-config/peerOrganizations/hospital.gcep.local/peers/peer0.hospital.gcep.local/tls/ca.crt" \
    peer lifecycle chaincode package /tmp/${CC_NAME}.tar.gz \
        --path "/opt/gopath/src/${CC_NAME}" \
        --lang "${CC_LANG}" --label "${CC_NAME}_${CC_VERSION}"

for peer in peer0.hospital.gcep.local peer0.regulator.gcep.local; do
    if ! docker ps --format '{{.Names}}' | grep -q "^${peer}\$"; then continue; fi
    org_dom="${peer#peer*.}"
    org_short="${org_dom%%.*}"
    org_msp="$(echo ${org_short} | awk '{print toupper(substr($0,1,1))substr($0,2)}')MSP"
    user="Admin@${org_dom}"

    echo "[deploy] installing on ${peer}…"
    peer_exec "$peer" "$org_msp" "$user" "" \
        peer lifecycle chaincode install /tmp/${CC_NAME}.tar.gz

    PKG_ID="$(peer_exec "$peer" "$org_msp" "$user" "" \
        peer lifecycle chaincode queryinstalled \
        | grep "${CC_NAME}_${CC_VERSION}" | awk '{print $3}' | sed 's/,$//')"
    echo "[deploy]   package id: ${PKG_ID}"

    echo "[deploy] approving on ${peer}…"
    peer_exec "$peer" "$org_msp" "$user" "" \
        peer lifecycle chaincode approveformyorg \
            -o "${ORDERER}" --tls --cafile "${ORDERER_TLS_CA}" \
            --channelID "${CHANNEL}" --name "${CC_NAME}" \
            --version "${CC_VERSION}" --package-id "${PKG_ID}" \
            --sequence "${CC_SEQUENCE}" \
            --signature-policy "${ENDORSEMENT}"
done

echo "[deploy] committing chaincode definition…"
peer_exec peer0.hospital.gcep.local HospitalMSP "Admin@hospital.gcep.local" "" \
    peer lifecycle chaincode commit \
        -o "${ORDERER}" --tls --cafile "${ORDERER_TLS_CA}" \
        --channelID "${CHANNEL}" --name "${CC_NAME}" \
        --version "${CC_VERSION}" --sequence "${CC_SEQUENCE}" \
        --signature-policy "${ENDORSEMENT}" \
        --peerAddresses peer0-hospital:7051 \
        --tlsRootCertFiles "/etc/hyperledger/fabric/crypto-config/peerOrganizations/hospital.gcep.local/peers/peer0.hospital.gcep.local/tls/ca.crt" \
        --peerAddresses peer0-regulator:7051 \
        --tlsRootCertFiles "/etc/hyperledger/fabric/crypto-config/peerOrganizations/regulator.gcep.local/peers/peer0.regulator.gcep.local/tls/ca.crt"

echo "[deploy] DONE — chaincode '${CC_NAME}' v${CC_VERSION} live on '${CHANNEL}'."
