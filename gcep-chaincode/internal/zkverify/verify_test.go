package zkverify

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests validate the wrapper around gnark, not gnark itself.
// Real Prove/Verify roundtrips live in gcep/circuits/own_circuit_test.go,
// which generates the proofs this code ultimately consumes.

func TestNewFromBytes_Garbage(t *testing.T) {
	_, err := NewFromBytes([]byte{0x00, 0x01, 0x02})
	require.Error(t, err, "garbage VK must not parse")
}

func TestVerify_BadHex(t *testing.T) {
	v := &Verifier{}
	err := v.Verify("not-hex", PublicInputs{
		ID:         []byte{0x01},
		BID:        []byte{0x01},
		CommitHash: []byte{0x01},
	})
	require.Error(t, err)
}

func TestBuildWitness_EmptyInputs(t *testing.T) {
	_, err := buildPublicWitness(PublicInputs{
		ID:         nil,
		BID:        []byte{0x01},
		CommitHash: []byte{0x01},
	})
	require.Error(t, err)
}
