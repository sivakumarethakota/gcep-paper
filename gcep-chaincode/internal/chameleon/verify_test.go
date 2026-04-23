package chameleon

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// helper — build a chameleon keypair and a valid (c, m, r) tuple, return all
// in hex form ready for Verify().
func freshTriple(t *testing.T) (pkHex, cHex, mHex, rHex string) {
	t.Helper()

	mod := fr.Modulus()

	// trapdoor x ←$ Z_q
	x, err := rand.Int(rand.Reader, mod)
	require.NoError(t, err)

	// y = g^x
	var g bn254.G1Affine
	_, _, gJac, _ := bn254.Generators()
	g.FromJacobian(&gJac)

	var yJac bn254.G1Jac
	yJac.FromAffine(&g).ScalarMultiplication(&yJac, x)
	var y bn254.G1Affine
	y.FromJacobian(&yJac)

	// pick m, r ←$ Z_q
	m, _ := rand.Int(rand.Reader, mod)
	r, _ := rand.Int(rand.Reader, mod)

	// c = g^m * y^r
	var gm, yr, c bn254.G1Jac
	gm.FromAffine(&g).ScalarMultiplication(&gm, m)
	yr.FromAffine(&y).ScalarMultiplication(&yr, r)
	c.Set(&gm).AddAssign(&yr)
	var cAff bn254.G1Affine
	cAff.FromJacobian(&c)

	return hex.EncodeToString(y.Marshal()),
		hex.EncodeToString(cAff.Marshal()),
		hex.EncodeToString(m.Bytes()),
		hex.EncodeToString(r.Bytes())
}

func TestVerify_HappyPath(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshTriple(t)
	pk, err := LoadPublicKey(pkHex)
	require.NoError(t, err)

	ok, err := pk.Verify(cHex, mHex, rHex)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestVerify_TamperedR(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshTriple(t)
	pk, _ := LoadPublicKey(pkHex)

	// Flip the last byte of r.
	rRaw, _ := hex.DecodeString(rHex)
	rRaw[len(rRaw)-1] ^= 0xFF
	tamperedR := hex.EncodeToString(rRaw)

	ok, err := pk.Verify(cHex, mHex, tamperedR)
	require.NoError(t, err)
	require.False(t, ok, "tampered r must not verify")
}

func TestVerify_TamperedM(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshTriple(t)
	pk, _ := LoadPublicKey(pkHex)

	mInt, _ := new(big.Int).SetString(mHex, 16)
	mInt.Add(mInt, big.NewInt(1))
	tamperedM := hex.EncodeToString(mInt.Bytes())

	ok, err := pk.Verify(cHex, tamperedM, rHex)
	require.NoError(t, err)
	require.False(t, ok, "tampered m must not verify")
}

func TestLoadPublicKey_Garbage(t *testing.T) {
	_, err := LoadPublicKey("not-hex")
	require.Error(t, err)

	_, err = LoadPublicKey("00112233")
	require.Error(t, err, "wrong-length bytes should fail")
}
