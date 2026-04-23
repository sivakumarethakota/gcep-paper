package store

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	types "github.com/yourorg/gcep/chaincode/internal/types"
	"github.com/yourorg/gcep/chaincode/test"
)

func TestCommitmentKey_Lowercase(t *testing.T) {
	require.Equal(t,
		CommitmentKey("ABCDEF"),
		CommitmentKey("abcdef"),
		"key derivation must be case-insensitive",
	)
}

func TestPutGet_Roundtrip(t *testing.T) {
	ctx := test.NewMockContext()

	cmt, err := types.NewCommitment(
		"abcd1234",
		42,
		"role:cardiologist",
		"deadbeef",
		"cafe1234",
		"feed5678",
		"HospitalMSP/cn=alice",
	)
	require.NoError(t, err)

	require.NoError(t, Put(ctx, cmt))

	got, err := Get(ctx, "abcd1234")
	require.NoError(t, err)
	require.Equal(t, cmt.C, got.C)
	require.Equal(t, cmt.Status, got.Status)
	require.Equal(t, cmt.Policy, got.Policy)
}

func TestGet_NotFound(t *testing.T) {
	ctx := test.NewMockContext()
	_, err := Get(ctx, "00000000")
	require.True(t, errors.Is(err, types.ErrCommitmentNotFound))
}

func TestExists(t *testing.T) {
	ctx := test.NewMockContext()

	exists, err := Exists(ctx, "abcd1234")
	require.NoError(t, err)
	require.False(t, exists)

	cmt, _ := types.NewCommitment("abcd1234", 1, "p", "00", "00", "00", "alice")
	require.NoError(t, Put(ctx, cmt))

	exists, err = Exists(ctx, "abcd1234")
	require.NoError(t, err)
	require.True(t, exists)
}
