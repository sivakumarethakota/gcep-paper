// Package circuits defines the R_own Groth16 circuit used by GCEP's Erasure
// protocol (Algorithm 3, Phase A). A successful proof convinces the chaincode
// verifier that the prover holds the private key that signed commitment c at
// write time — without revealing the key or public key.
//
// Circuit statement (informally):
//
//   Public inputs:
//     ID         — commitment to the patient identity, = MiMC(PubX, PubY)
//     BID        — block identifier (uint64 packed into a field element)
//     CommitHash — MiMC digest of the chameleon commitment c
//
//   Private witness:
//     PubKey.A.(X,Y) — EdDSA public key on Baby JubJub (embedded in BN254)
//     Sig.{R, S}     — EdDSA signature over the message
//     Msg            — canonical message = MiMC(CommitHash, BID)
//
//   Asserts:
//     1.  MiMC(PubX, PubY) == ID
//     2.  Msg              == MiMC(CommitHash, BID)
//     3.  EdDSA.Verify(PubKey, Sig, Msg) holds
//
// Chosen curves:
//   Outer (SNARK): BN254, for compatibility with Ethereum-style verifiers and
//   for gnark's mature Groth16 back-end.
//   Inner (EdDSA): Baby JubJub twisted-Edwards curve embedded in BN254's
//   scalar field. This lets us do signature verification in-circuit at
//   reasonable cost.
//
// This package intentionally exports OwnCircuit as a plain struct so the
// `cmd/setup` tool and the test file can reuse it.
package circuits

import (
	tedwards "github.com/consensys/gnark-crypto/ecc/twistededwards"
	"github.com/consensys/gnark/frontend"
	twistededwardsstd "github.com/consensys/gnark/std/algebra/native/twistededwards"
	"github.com/consensys/gnark/std/hash/mimc"
	"github.com/consensys/gnark/std/signature/eddsa"
)

// OwnCircuit is the R_own relation. Tagged fields with `gnark:",public"` form
// the statement; untagged fields form the witness.
type OwnCircuit struct {
	// Public inputs
	ID         frontend.Variable `gnark:",public"`
	BID        frontend.Variable `gnark:",public"`
	CommitHash frontend.Variable `gnark:",public"`

	// Private witness
	PubKey eddsa.PublicKey
	Sig    eddsa.Signature
	Msg    frontend.Variable
}

// Define implements the gnark frontend.Circuit interface.
func (c *OwnCircuit) Define(api frontend.API) error {
	// 1. Identity binding: ID = MiMC(PubX, PubY)
	h1, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}
	h1.Write(c.PubKey.A.X, c.PubKey.A.Y)
	computedID := h1.Sum()
	api.AssertIsEqual(c.ID, computedID)

	// 2. Message canonicalisation: Msg = MiMC(CommitHash, BID)
	h2, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}
	h2.Write(c.CommitHash, c.BID)
	computedMsg := h2.Sum()
	api.AssertIsEqual(c.Msg, computedMsg)

	// 3. Signature verification on the embedded Baby JubJub curve.
	curve, err := twistededwardsstd.NewEdCurve(api, tedwards.BN254)
	if err != nil {
		return err
	}
	h3, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}
	return eddsa.Verify(curve, c.Sig, c.Msg, c.PubKey, &h3)
}
