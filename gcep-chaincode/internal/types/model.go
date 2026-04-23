package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Status is the lifecycle state of a Commitment.
//
// Transitions:
//
//	(none) -- Write          --> ACTIVE
//	ACTIVE -- Erase          --> PENDING_ERASURE
//	PENDING_ERASURE -- FinaliseErase --> ERASED
//
// All other transitions are rejected by the contract.
type Status string

const (
	StatusActive          Status = "ACTIVE"
	StatusPendingErasure  Status = "PENDING_ERASURE"
	StatusErased          Status = "ERASED"
)

// Policy is an opaque, contract-enforced expression governing read access to
// the off-chain blob this commitment points at. The chaincode does not
// evaluate it (the hospital's application server does); we store and
// surface it on `GetCommitment` so external auditors can inspect.
//
// Format is intentionally a single string — typically a CP-ABE policy like
// "role:cardiologist AND org:hospital" or a CEL expression. Keep it under
// 1 KB; otherwise on-chain footprint per commitment grows uncomfortably.
type Policy string

// Commitment is the canonical on-chain record. It is keyed by the chameleon
// hash bytes (hex-encoded as the world-state key for CouchDB compatibility).
//
// Fields M and R together form the chameleon pre-image:  CH(M, R) == C.
// On Erase / FinaliseErase, M and R change but C must remain byte-identical
// (invariant I2 — block-structure preservation).
type Commitment struct {
	C          string  `json:"c"`           // chameleon hash, hex-encoded
	BID        uint64  `json:"bid"`         // block identifier (write-time)
	Policy     Policy  `json:"policy"`      // access policy expression
	M          string  `json:"m"`           // pre-image message,  hex
	R          string  `json:"r"`           // pre-image randomness, hex
	SigPatient string  `json:"sigPatient"`  // patient Schnorr signature, hex
	Status     Status  `json:"status"`
	WrittenAt  int64   `json:"writtenAt"`   // unix nanos
	UpdatedAt  int64   `json:"updatedAt"`   // unix nanos; tracks status changes
	WrittenBy  string  `json:"writtenBy"`   // submitting client identity (MSPID + cert hash)
}

// NewCommitment is the constructor used by the Write transaction. We
// validate field shapes here so the contract method stays focused on
// state-machine logic.
func NewCommitment(c string, bid uint64, policy Policy, sigP, m, r string, writtenBy string) (*Commitment, error) {
	if err := validateHex(c, "c"); err != nil {
		return nil, err
	}
	if err := validateHex(m, "m"); err != nil {
		return nil, err
	}
	if err := validateHex(r, "r"); err != nil {
		return nil, err
	}
	if err := validateHex(sigP, "sigPatient"); err != nil {
		return nil, err
	}
	if len(policy) == 0 {
		return nil, fmt.Errorf("policy must not be empty")
	}
	if len(policy) > 1024 {
		return nil, fmt.Errorf("policy too long: %d > 1024 bytes", len(policy))
	}

	now := time.Now().UnixNano()
	return &Commitment{
		C:          strings.ToLower(c),
		BID:        bid,
		Policy:     policy,
		M:          strings.ToLower(m),
		R:          strings.ToLower(r),
		SigPatient: strings.ToLower(sigP),
		Status:     StatusActive,
		WrittenAt:  now,
		UpdatedAt:  now,
		WrittenBy:  writtenBy,
	}, nil
}

// Marshal returns the canonical JSON encoding stored in world state. We use
// json.Marshal directly (not protobuf) so a CouchDB rich-query auditor can
// read the state without recompiling.
func (c *Commitment) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalCommitment is the inverse of Marshal.
func UnmarshalCommitment(b []byte) (*Commitment, error) {
	var c Commitment
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("unmarshal commitment: %w", err)
	}
	return &c, nil
}

// -----------------------------------------------------------------------------
// Event payloads
// -----------------------------------------------------------------------------

// EventWritten is emitted on every successful Write.
type EventWritten struct {
	C         string `json:"c"`
	BID       uint64 `json:"bid"`
	WrittenBy string `json:"writtenBy"`
	Timestamp int64  `json:"timestamp"`
}

// EventEraseRequested is emitted on Erase. The committee daemon subscribes
// to this event over Fabric's `chaincode-events` channel.
type EventEraseRequested struct {
	C         string `json:"c"`
	BID       uint64 `json:"bid"`
	M         string `json:"m"`         // current pre-image message
	R         string `json:"r"`         // current pre-image randomness
	Requester string `json:"requester"` // client identity
	Timestamp int64  `json:"timestamp"`
}

// EventErased is emitted on successful FinaliseErase. This is the audit log.
type EventErased struct {
	C         string `json:"c"`
	BID       uint64 `json:"bid"`
	MOld      string `json:"mOld"`
	MNew      string `json:"mNew"`
	Finaliser string `json:"finaliser"`
	Timestamp int64  `json:"timestamp"`
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func validateHex(s, name string) error {
	if len(s) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	if len(s)%2 != 0 {
		return fmt.Errorf("%s must be even-length hex (got %d chars)", name, len(s))
	}
	for i, ch := range strings.ToLower(s) {
		isHex := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')
		if !isHex {
			return fmt.Errorf("%s contains non-hex character at index %d: %q", name, i, ch)
		}
	}
	return nil
}
