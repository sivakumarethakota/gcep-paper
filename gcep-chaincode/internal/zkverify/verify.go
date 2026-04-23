// Package zkverify wraps gnark's Groth16 verifier. The Erase transaction
// calls Verify() to validate the patient's proof of ownership for the
// commitment they want erased.
//
// At deploy time, the verifying key is loaded from disk (or, in production,
// from a Fabric channel-config blob). Once loaded, it never changes for a
// given chaincode version — Groth16 keys are circuit-specific and the R_own
// circuit is fixed.
package zkverify

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
)

// Verifier holds a parsed Groth16 verifying key for one circuit (R_own).
type Verifier struct {
	vk groth16.VerifyingKey
}

// NewFromBytes parses a serialised verifying key (as written by gnark's
// VerifyingKey.WriteRawTo). The blob is typically pinned in a chaincode
// channel-config record so all peers verify against the same key.
func NewFromBytes(raw []byte) (*Verifier, error) {
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("read verifying key: %w", err)
	}
	return &Verifier{vk: vk}, nil
}

// PublicInputs is the strongly-typed public-input bundle for R_own. These
// values must match the order in `circuits/own_circuit.go` exactly:
//
//	1. ID         — patient identity commitment
//	2. BID        — block identifier
//	3. CommitHash — MiMC digest of the chameleon commitment c
type PublicInputs struct {
	ID         []byte
	BID        []byte
	CommitHash []byte
}

// Verify returns nil iff `proofHex` is a valid Groth16 proof for the public
// inputs `pi` under the circuit's verifying key.
//
// On rejection, returns an error wrapping the underlying gnark message.
// Callers in chaincode should map this to gcep.ErrZKProofRejected so the
// transaction error shape stays predictable.
func (v *Verifier) Verify(proofHex string, pi PublicInputs) error {
	proofBytes, err := hex.DecodeString(proofHex)
	if err != nil {
		return fmt.Errorf("decode proof hex: %w", err)
	}

	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		return fmt.Errorf("read proof: %w", err)
	}

	w, err := buildPublicWitness(pi)
	if err != nil {
		return fmt.Errorf("build witness: %w", err)
	}

	if err := groth16.Verify(proof, v.vk, w); err != nil {
		return fmt.Errorf("groth16 verify: %w", err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

func buildPublicWitness(pi PublicInputs) (witness.Witness, error) {
	if len(pi.ID) == 0 || len(pi.BID) == 0 || len(pi.CommitHash) == 0 {
		return nil, errors.New("public inputs must all be non-empty")
	}

	w, err := witness.New(ecc.BN254.ScalarField())
	if err != nil {
		return nil, err
	}

	// Stream the three public inputs in declaration order. gnark's witness
	// API is positional — the exporter must match the circuit struct order.
	ch := make(chan any, 3)
	ch <- pi.ID
	ch <- pi.BID
	ch <- pi.CommitHash
	close(ch)

	if err := w.Fill(3, 0, ch); err != nil {
		return nil, fmt.Errorf("fill witness: %w", err)
	}
	return w, nil
}
