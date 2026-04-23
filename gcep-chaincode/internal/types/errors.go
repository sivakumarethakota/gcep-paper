package types

import "errors"

// Sentinel errors returned by the contract methods. Tests should use
// errors.Is to match these, not string equality.
var (
	ErrCommitmentExists       = errors.New("commitment already exists")
	ErrCommitmentNotFound     = errors.New("commitment not found")
	ErrInvalidStatus          = errors.New("commitment in invalid status for this operation")
	ErrZKProofRejected        = errors.New("groth16 verify rejected the proof")
	ErrCommitteeSigRejected   = errors.New("threshold committee signature rejected")
	ErrChameleonMismatch      = errors.New("chameleon collision check failed: g^m' * y^r' != c")
	ErrAlreadyErased          = errors.New("commitment is already erased")
	ErrPolicyTooLarge         = errors.New("policy exceeds 1024 bytes")
	ErrUnauthorisedSubmitter  = errors.New("submitting identity not permitted for this operation")
)
