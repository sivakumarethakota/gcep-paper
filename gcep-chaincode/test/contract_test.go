package test

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"

	gcep "github.com/yourorg/gcep/chaincode"
	types "github.com/yourorg/gcep/chaincode/internal/types"
)

// helper — generate a chameleon keypair and a valid (c, m, r) tuple in hex
// form, plus the public key in hex. Mirrors the chameleon test helper.
func freshChameleonTriple(t *testing.T) (pkHex, cHex, mHex, rHex string) {
	t.Helper()

	mod := fr.Modulus()

	x, _ := rand.Int(rand.Reader, mod)
	// var g bn254.G1Affine -- replaced below
	_, _, g, _ := bn254.Generators()
	// g initialised via Generators()

	var yJac bn254.G1Jac
	yJac.FromAffine(&g).ScalarMultiplication(&yJac, x)
	var y bn254.G1Affine
	y.FromJacobian(&yJac)

	m, _ := rand.Int(rand.Reader, mod)
	r, _ := rand.Int(rand.Reader, mod)

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

// TODO(integration): when Erase round-trip tests land, restore the
// trapdoorCollision helper that takes (xInt, mInt, rInt, mPrimeInt) and
// returns r' such that g^m' * y^r' == g^m * y^r. The math is in §3.4.1 of
// the paper; r' = r + x⁻¹(m − m')  mod q.

// --- Test cases ---------------------------------------------------------------

func TestWrite_HappyPath(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshChameleonTriple(t)

	contract := &gcep.SmartContract{}
	ctx := NewMockContext()
	require.NoError(t, contract.Initialise(ctx, pkHex, ""))

	got, err := contract.Write(ctx, cHex, "100", "role:cardiologist", "deadbeef", mHex, rHex)
	require.NoError(t, err)
	require.Equal(t, cHex, got)

	// State persisted?
	cmt, err := contract.GetCommitment(ctx, cHex)
	require.NoError(t, err)
	require.Equal(t, types.StatusActive, cmt.Status)
	require.Equal(t, uint64(100), cmt.BID)

	// Event emitted?
	events := ctx.Events()
	require.Len(t, events, 1)
	require.Equal(t, "Written", events[0].Name)
}

func TestWrite_DoubleWriteRejected(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshChameleonTriple(t)
	contract := &gcep.SmartContract{}
	ctx := NewMockContext()
	require.NoError(t, contract.Initialise(ctx, pkHex, ""))

	_, err := contract.Write(ctx, cHex, "1", "p", "00", mHex, rHex)
	require.NoError(t, err)

	_, err = contract.Write(ctx, cHex, "1", "p", "00", mHex, rHex)
	require.True(t, errors.Is(err, types.ErrCommitmentExists),
		"second Write on same c must be rejected, got: %v", err)
}

func TestWrite_ChameleonMismatchRejected(t *testing.T) {
	pkHex, cHex, _, _ := freshChameleonTriple(t)
	contract := &gcep.SmartContract{}
	ctx := NewMockContext()
	require.NoError(t, contract.Initialise(ctx, pkHex, ""))

	// Submit garbage m, r — defensive Verify must reject.
	_, err := contract.Write(ctx, cHex, "1", "p", "00", "deadbeef", "cafef00d")
	require.True(t, errors.Is(err, types.ErrChameleonMismatch),
		"mismatched (m, r) must be rejected, got: %v", err)
}

func TestGetCommitment_NotFound(t *testing.T) {
	contract := &gcep.SmartContract{}
	ctx := NewMockContext()
	_, err := contract.GetCommitment(ctx, "00")
	require.True(t, errors.Is(err, types.ErrCommitmentNotFound))
}

func TestFinaliseErase_RequiresPendingStatus(t *testing.T) {
	pkHex, cHex, mHex, rHex := freshChameleonTriple(t)
	contract := &gcep.SmartContract{}
	ctx := NewMockContext()
	require.NoError(t, contract.Initialise(ctx, pkHex, ""))

	_, err := contract.Write(ctx, cHex, "1", "p", "00", mHex, rHex)
	require.NoError(t, err)

	// Skip Erase; FinaliseErase must refuse on ACTIVE.
	err = contract.FinaliseErase(ctx, cHex, mHex, rHex, "deadbeef", "feed")
	require.True(t, errors.Is(err, types.ErrInvalidStatus),
		"FinaliseErase on ACTIVE commitment must be rejected, got: %v", err)
}

// NOTE: a full Erase → FinaliseErase round-trip needs a real Groth16 proof,
// which lives in `gcep/circuits`. That integration test belongs in a separate
// suite that runs `circuits/cmd/setup` first and consumes the resulting key.
// The unit tests above cover every contract path that doesn't require a proof.
