// Package chameleon implements just enough of the chameleon-hash construction
// to let chaincode VERIFY a collision. It does NOT implement collision
// finding (that requires the trapdoor and runs on the committee daemon).
//
// Construction (DL-based, Krawczyk-Rabin / Ateniese form on BN254 G1):
//
//	CH(m, r) = g^m * y^r          where g is the BN254 G1 generator
//	                              and y = g^x is the public key.
//
// At deploy time, chaincode embeds y. Anyone can verify; nobody but the
// trapdoor holder can find collisions.
package chameleon

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// PublicKey is the chameleon public key y = g^x in BN254 G1 (affine form).
type PublicKey struct {
	Y bn254.G1Affine
}

// LoadPublicKey parses a hex-encoded compressed G1 point. The committee's
// DKG output should be persisted in this format.
func LoadPublicKey(hexBytes string) (*PublicKey, error) {
	raw, err := hex.DecodeString(hexBytes)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	var y bn254.G1Affine
	if _, err := y.SetBytes(raw); err != nil {
		return nil, fmt.Errorf("g1 unmarshal: %w", err)
	}
	if !y.IsOnCurve() {
		return nil, errors.New("public key point not on curve")
	}
	return &PublicKey{Y: y}, nil
}

// Verify returns true iff CH(m, r) == c, where m and r are field elements
// (mod the BN254 scalar field) and c is the committed G1 point.
//
// All inputs are hex-encoded for chaincode-interface compatibility.
func (pk *PublicKey) Verify(cHex, mHex, rHex string) (bool, error) {
	c, err := decodeG1(cHex)
	if err != nil {
		return false, fmt.Errorf("decode c: %w", err)
	}
	m, err := decodeScalar(mHex)
	if err != nil {
		return false, fmt.Errorf("decode m: %w", err)
	}
	r, err := decodeScalar(rHex)
	if err != nil {
		return false, fmt.Errorf("decode r: %w", err)
	}

	// Compute g^m and y^r, sum, compare to c.
	var g bn254.G1Affine
	_, _, gJac, _ := bn254.Generators()
	g.FromJacobian(&gJac)

	var gm, yr, sum bn254.G1Jac
	gm.FromAffine(&g).ScalarMultiplication(&gm, m)
	yr.FromAffine(&pk.Y).ScalarMultiplication(&yr, r)
	sum.Set(&gm).AddAssign(&yr)

	var sumAff bn254.G1Affine
	sumAff.FromJacobian(&sum)

	return sumAff.Equal(&c), nil
}

// MustVerify wraps Verify and panics on parsing errors. Use only in tests.
func (pk *PublicKey) MustVerify(cHex, mHex, rHex string) bool {
	ok, err := pk.Verify(cHex, mHex, rHex)
	if err != nil {
		panic(err)
	}
	return ok
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

func decodeG1(s string) (bn254.G1Affine, error) {
	var p bn254.G1Affine
	raw, err := hex.DecodeString(s)
	if err != nil {
		return p, err
	}
	if _, err := p.SetBytes(raw); err != nil {
		return p, err
	}
	if !p.IsOnCurve() {
		return p, errors.New("point not on curve")
	}
	return p, nil
}

func decodeScalar(s string) (*big.Int, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	x := new(big.Int).SetBytes(raw)
	// Reduce mod r so callers don't have to.
	mod := fr.Modulus()
	return x.Mod(x, mod), nil
}
