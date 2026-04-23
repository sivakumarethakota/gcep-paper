// Package gcep is the Hyperledger Fabric chaincode for the GDPR-Compliant
// Erasure Protocol described in §3.6 of the paper.
//
// Three writeable transactions and one read-only query:
//
//   - Write           — Algorithm 1   (record a chameleon commitment)
//   - Erase           — Algorithm 3 Phase B  (validate ZK proof, mark PENDING)
//   - FinaliseErase   — Algorithm 3 Phase D  (validate threshold sig + collision)
//   - GetCommitment   — read-only world-state query
//
// All four enforce strict argument validation and status-machine transitions.
// Every transition emits a chaincode event so off-chain services (committee
// daemons, audit pipelines) can react without polling.
package gcep

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"

	"github.com/yourorg/gcep/chaincode/internal/chameleon"
	"github.com/yourorg/gcep/chaincode/internal/store"
	"github.com/yourorg/gcep/chaincode/internal/zkverify"
)

// SmartContract is the Fabric contract type. It carries pre-loaded verifier
// instances so each transaction call doesn't pay the parsing cost.
type SmartContract struct {
	contractapi.Contract

	// Loaded once at chaincode init via Initialise(). nil until then.
	chameleonPK *chameleon.PublicKey
	zkVerifier  *zkverify.Verifier
}

// Initialise is called once per chaincode instance to load the trust roots.
// In production these blobs come from a Fabric channel-config record so they
// are governed by the channel's `Admins` policy and survive chaincode upgrade.
//
// Args:
//
//	chameleonPubKeyHex — hex-encoded compressed BN254 G1 point (y = g^x)
//	verifyingKeyHex    — hex-encoded gnark Groth16 verifying key
func (s *SmartContract) Initialise(ctx contractapi.TransactionContextInterface, chameleonPubKeyHex, verifyingKeyHex string) error {
	pk, err := chameleon.LoadPublicKey(chameleonPubKeyHex)
	if err != nil {
		return fmt.Errorf("load chameleon pk: %w", err)
	}
	vkRaw, err := hex.DecodeString(verifyingKeyHex)
	if err != nil {
		return fmt.Errorf("decode vk hex: %w", err)
	}
	v, err := zkverify.NewFromBytes(vkRaw)
	if err != nil {
		return fmt.Errorf("load zk verifier: %w", err)
	}
	s.chameleonPK = pk
	s.zkVerifier = v
	return nil
}

// -----------------------------------------------------------------------------
// Write — Algorithm 1
// -----------------------------------------------------------------------------

// Write records a fresh commitment in world state. Inputs:
//
//	c       — chameleon hash, hex-encoded BN254 G1 point
//	bid     — block identifier (decimal uint64 string)
//	policy  — access-policy expression (e.g. CP-ABE policy)
//	sigP    — patient Schnorr signature over (c||bid), hex
//	m       — chameleon pre-image message, hex (≤ 32 bytes)
//	r       — chameleon pre-image randomness, hex (≤ 32 bytes)
//
// Returns the canonical commitment key (the hex form of c).
//
// Side effects:
//
//	- Persists Commitment{...Status: ACTIVE} at world-state key
//	  "commitment\u0000<c>".
//	- Emits chaincode event "Written".
//
// Errors:
//
//	- ErrCommitmentExists if the key is already populated.
//	- Validation errors from NewCommitment for malformed inputs.
//	- ErrChameleonMismatch if CH(m, r) != c (defensive — the client
//	  should have computed c from the same m, r, but we verify so a
//	  buggy client cannot poison world state).
func (s *SmartContract) Write(ctx contractapi.TransactionContextInterface, c, bidStr, policy, sigP, m, r string) (string, error) {
	bid, err := strconv.ParseUint(bidStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse bid: %w", err)
	}

	exists, err := store.Exists(ctx, c)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrCommitmentExists
	}

	// Defensive chameleon check — confirm client-supplied c really is
	// CH(m, r). Cheap (~1 ms on BN254) and prevents a class of bugs.
	if s.chameleonPK == nil {
		return "", errors.New("contract not initialised: call Initialise first")
	}
	ok, err := s.chameleonPK.Verify(c, m, r)
	if err != nil {
		return "", fmt.Errorf("chameleon verify on write: %w", err)
	}
	if !ok {
		return "", ErrChameleonMismatch
	}

	submitter, err := submittingIdentity(ctx)
	if err != nil {
		return "", err
	}

	cmt, err := NewCommitment(c, bid, Policy(policy), sigP, m, r, submitter)
	if err != nil {
		return "", fmt.Errorf("build commitment: %w", err)
	}

	if err := store.Put(ctx, cmt); err != nil {
		return "", err
	}

	if err := emitEvent(ctx, "Written", EventWritten{
		C:         cmt.C,
		BID:       cmt.BID,
		WrittenBy: submitter,
		Timestamp: time.Now().UnixNano(),
	}); err != nil {
		return "", err
	}
	return cmt.C, nil
}

// -----------------------------------------------------------------------------
// Erase — Algorithm 3 Phase B
// -----------------------------------------------------------------------------

// Erase verifies the patient's Groth16 proof of ownership and flips the
// commitment to PENDING_ERASURE. The committee daemon picks up the
// EraseRequested event and runs the threshold session that produces
// (m', r', σ_C) for FinaliseErase.
//
// Inputs:
//
//	c        — commitment key (hex)
//	bidStr   — block identifier (decimal uint64), must match world state
//	zkProof  — Groth16 proof, hex-encoded
//
// The proof's public inputs are derived as:
//
//	ID         = the submitting client identity hash
//	BID        = bid (8-byte big-endian)
//	CommitHash = MiMC(c)   — but pre-computed here as just c bytes,
//	             since the circuit uses the same canonicalisation.
//
// Errors:
//
//	- ErrCommitmentNotFound if c is unknown.
//	- ErrInvalidStatus if the commitment is not ACTIVE.
//	- ErrZKProofRejected if Groth16.Verify fails.
func (s *SmartContract) Erase(ctx contractapi.TransactionContextInterface, c, bidStr, zkProof string) error {
	bid, err := strconv.ParseUint(bidStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse bid: %w", err)
	}

	cmt, err := store.Get(ctx, c)
	if err != nil {
		return err
	}
	if cmt.Status != StatusActive {
		return fmt.Errorf("%w: status=%s", ErrInvalidStatus, cmt.Status)
	}
	if cmt.BID != bid {
		return fmt.Errorf("bid mismatch: world state has %d, request says %d", cmt.BID, bid)
	}

	if s.zkVerifier == nil {
		return errors.New("contract not initialised: call Initialise first")
	}

	submitter, err := submittingIdentity(ctx)
	if err != nil {
		return err
	}

	pi := zkverify.PublicInputs{
		ID:         []byte(submitter),
		BID:        uint64ToBytes(bid),
		CommitHash: hexMustDecode(c),
	}
	if err := s.zkVerifier.Verify(zkProof, pi); err != nil {
		return fmt.Errorf("%w: %v", ErrZKProofRejected, err)
	}

	cmt.Status = StatusPendingErasure
	cmt.UpdatedAt = time.Now().UnixNano()
	if err := store.Put(ctx, cmt); err != nil {
		return err
	}

	return emitEvent(ctx, "EraseRequested", EventEraseRequested{
		C:         cmt.C,
		BID:       cmt.BID,
		M:         cmt.M,
		R:         cmt.R,
		Requester: submitter,
		Timestamp: cmt.UpdatedAt,
	})
}

// -----------------------------------------------------------------------------
// FinaliseErase — Algorithm 3 Phase D
// -----------------------------------------------------------------------------

// FinaliseErase consumes the committee's threshold output (m', r', σ_C),
// verifies that (m', r') is a valid chameleon collision for the existing c,
// confirms the committee signature, then rewrites the pre-image and marks
// the commitment ERASED.
//
// Inputs:
//
//	c          — commitment key (hex)
//	mPrime     — new pre-image message (hex)
//	rPrime     — new pre-image randomness (hex)
//	sigC       — threshold-BLS signature, hex
//	sigCMsg    — canonical message signed: H(c || mPrime || rPrime), hex
//
// We pass `sigCMsg` separately rather than recomputing in chaincode so that
// the signature scheme can evolve (e.g. domain separation) without a
// chaincode upgrade. The committee daemon must compute it identically.
//
// Note: threshold-BLS signature verification is intentionally OUT OF SCOPE
// for this skeleton. In production, plug a verifier from `drand/kyber`'s
// share package here. We document the expected interface and leave a TODO.
//
// Errors:
//
//	- ErrCommitmentNotFound
//	- ErrInvalidStatus      if not in PENDING_ERASURE
//	- ErrChameleonMismatch  if g^m' * y^r' != c
//	- ErrCommitteeSigRejected (when threshold verify is wired in)
func (s *SmartContract) FinaliseErase(ctx contractapi.TransactionContextInterface, c, mPrime, rPrime, sigC, sigCMsg string) error {
	cmt, err := store.Get(ctx, c)
	if err != nil {
		return err
	}
	if cmt.Status == StatusErased {
		return ErrAlreadyErased
	}
	if cmt.Status != StatusPendingErasure {
		return fmt.Errorf("%w: status=%s", ErrInvalidStatus, cmt.Status)
	}

	if s.chameleonPK == nil {
		return errors.New("contract not initialised: call Initialise first")
	}

	// Must hold: CH(m', r') == c.  (Invariant I2 — block-structure preservation.)
	ok, err := s.chameleonPK.Verify(c, mPrime, rPrime)
	if err != nil {
		return fmt.Errorf("chameleon verify on finalise: %w", err)
	}
	if !ok {
		return ErrChameleonMismatch
	}

	// TODO(committee-bls): verify threshold-BLS signature sigC over sigCMsg.
	// For now, we require the signature field to be non-empty as a placeholder
	// and rely on Fabric endorsement (`AND(Hospital,Regulator)`) to gate the
	// transaction. Replace with `bls.Verify(aggPubKey, sigCMsg, sigC)`.
	if len(sigC) == 0 || len(sigCMsg) == 0 {
		return fmt.Errorf("%w: sigC and sigCMsg required", ErrCommitteeSigRejected)
	}

	finaliser, err := submittingIdentity(ctx)
	if err != nil {
		return err
	}

	mOld := cmt.M
	cmt.M = mPrime
	cmt.R = rPrime
	cmt.Status = StatusErased
	cmt.UpdatedAt = time.Now().UnixNano()
	if err := store.Put(ctx, cmt); err != nil {
		return err
	}

	return emitEvent(ctx, "Erased", EventErased{
		C:         cmt.C,
		BID:       cmt.BID,
		MOld:      mOld,
		MNew:      cmt.M,
		Finaliser: finaliser,
		Timestamp: cmt.UpdatedAt,
	})
}

// -----------------------------------------------------------------------------
// GetCommitment — read-only query
// -----------------------------------------------------------------------------

// GetCommitment returns the JSON-encoded commitment record for hash c.
// Returns ErrCommitmentNotFound if absent.
func (s *SmartContract) GetCommitment(ctx contractapi.TransactionContextInterface, c string) (*Commitment, error) {
	return store.Get(ctx, c)
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// submittingIdentity returns a deterministic identifier for the transaction
// submitter, of the form "<MSPID>/<sha256(cert) hex prefix>".
func submittingIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	mspid, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("get mspid: %w", err)
	}
	id, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("get client id: %w", err)
	}
	// GetID() already returns a stable string ("x509::CN=alice::CN=ca…").
	// We prefix with MSPID for readability.
	return mspid + "/" + id, nil
}

func emitEvent(ctx contractapi.TransactionContextInterface, name string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event %s: %w", name, err)
	}
	if err := ctx.GetStub().SetEvent(name, raw); err != nil {
		return fmt.Errorf("setevent %s: %w", name, err)
	}
	return nil
}

func uint64ToBytes(x uint64) []byte {
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = byte(x & 0xFF)
		x >>= 8
	}
	return out
}

func hexMustDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		// Should never happen — model.go's validateHex runs first.
		// We return nil so the caller sees a verify failure rather than
		// a panicking chaincode (panics taint world state for the tx).
		return nil
	}
	return b
}
