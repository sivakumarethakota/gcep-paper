package circuits

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	tedwards "github.com/consensys/gnark-crypto/ecc/twistededwards"
	eddsa_native "github.com/consensys/gnark-crypto/ecc/twistededwards/eddsa"
	hash_native "github.com/consensys/gnark-crypto/hash"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/signature/eddsa"
	"github.com/consensys/gnark/test"
)

// TestOwnCircuit_Satisfiability checks that a well-formed witness satisfies
// the constraint system. Runs under gnark/test which does not require a
// trusted setup — it's the fastest sanity check.
func TestOwnCircuit_Satisfiability(t *testing.T) {
	assert := test.NewAssert(t)
	assignment := buildValidAssignment(t)

	assert.SolvingSucceeded(&OwnCircuit{}, assignment, test.WithCurves(ecc.BN254))
}

// TestOwnCircuit_TamperedID fails if someone tries to prove ownership under a
// different patient ID than the one bound to the public key. Exercises P2.
func TestOwnCircuit_TamperedID(t *testing.T) {
	assert := test.NewAssert(t)
	assignment := buildValidAssignment(t)

	// Flip the ID; everything else stays valid. Circuit should reject.
	assignment.ID = new(big.Int).Add(assignment.ID.(*big.Int), big.NewInt(1))

	assert.SolvingFailed(&OwnCircuit{}, assignment, test.WithCurves(ecc.BN254))
}

// TestOwnCircuit_EndToEnd_Groth16 runs the full Setup / Prove / Verify loop.
// This is what the chaincode verifier will do in production.
func TestOwnCircuit_EndToEnd_Groth16(t *testing.T) {
	var circuit OwnCircuit

	t.Log("Compiling circuit…")
	start := time.Now()
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	t.Logf("constraints=%d  compiled in %s", cs.GetNbConstraints(), time.Since(start))

	t.Log("Groth16 setup (dev only — NOT for production)…")
	start = time.Now()
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Logf("setup took %s", time.Since(start))

	// Size of serialised VK — this is what chaincode has to carry.
	var vkBuf bytes.Buffer
	if _, err := vk.WriteRawTo(&vkBuf); err != nil {
		t.Fatalf("vk marshal: %v", err)
	}
	t.Logf("verifying key size = %d bytes", vkBuf.Len())

	assignment := buildValidAssignment(t)
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness: %v", err)
	}
	publicWitness, err := fullWitness.Public()
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}

	t.Log("Proving…")
	start = time.Now()
	proof, err := groth16.Prove(cs, pk, fullWitness)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proveTime := time.Since(start)

	var pBuf bytes.Buffer
	if _, err := proof.WriteRawTo(&pBuf); err != nil {
		t.Fatalf("proof marshal: %v", err)
	}
	t.Logf("prove took %s, proof size = %d bytes", proveTime, pBuf.Len())

	t.Log("Verifying…")
	start = time.Now()
	if err := groth16.Verify(proof, vk, publicWitness); err != nil {
		t.Fatalf("verify: %v", err)
	}
	t.Logf("verify took %s", time.Since(start))
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// buildValidAssignment generates a fresh EdDSA keypair on Baby JubJub,
// signs a canonical message, and packages everything into an OwnCircuit
// witness that should satisfy the constraint system.
func buildValidAssignment(t *testing.T) *OwnCircuit {
	t.Helper()

	// --- 1. Native keypair ----------------------------------------------------
	sk, err := eddsa_native.New(tedwards.BN254, rand.Reader)
	if err != nil {
		t.Fatalf("eddsa keygen: %v", err)
	}
	pk := sk.Public()
	pkBytes := pk.Bytes()

	// --- 2. Public inputs -----------------------------------------------------
	// ID = MiMC(PubX, PubY) using the same MiMC instance the circuit uses.
	hID := hash_native.MIMC_BN254.New()
	// PubKey bytes for Baby JubJub/BN254 are fixed-width coordinates; split:
	halfLen := len(pkBytes) / 2
	pubX := pkBytes[:halfLen]
	pubY := pkBytes[halfLen:]
	hID.Write(pubX)
	hID.Write(pubY)
	idBytes := hID.Sum(nil)

	commitHash := randomFieldElement()
	bid := big.NewInt(int64(time.Now().UnixNano()))

	// Msg = MiMC(CommitHash, BID)
	hMsg := hash_native.MIMC_BN254.New()
	hMsg.Write(paddedBytes(commitHash))
	hMsg.Write(paddedBytes(bid))
	msgBytes := hMsg.Sum(nil)

	// --- 3. Sign the canonical message ---------------------------------------
	hSign := hash_native.MIMC_BN254.New()
	sig, err := sk.Sign(msgBytes, hSign)
	if err != nil {
		t.Fatalf("eddsa sign: %v", err)
	}

	// --- 4. Populate the circuit assignment ----------------------------------
	assignment := &OwnCircuit{
		ID:         new(big.Int).SetBytes(idBytes),
		BID:        bid,
		CommitHash: commitHash,
		Msg:        new(big.Int).SetBytes(msgBytes),
	}
	assignment.PubKey.Assign(tedwards.BN254, pkBytes)
	assignment.Sig.Assign(tedwards.BN254, sig)

	return assignment
}

func randomFieldElement() *big.Int {
	q := ecc.BN254.ScalarField()
	x, _ := rand.Int(rand.Reader, q)
	return x
}

func paddedBytes(x *big.Int) []byte {
	// MiMC expects 32-byte-aligned BN254 field elements.
	b := x.Bytes()
	if len(b) >= 32 {
		return b[:32]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}
