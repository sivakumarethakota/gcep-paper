// Package store wraps Fabric's world-state interface with a typed API for
// Commitment objects. It also defines the canonical key layout, which is
// "commitment\u0000<hex>" — chosen so a CouchDB rich query like
// {"selector":{"_id":{"$regex":"^commitment\u0000"}}} returns every commitment.
package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"

	gcep "github.com/yourorg/gcep/chaincode"
)

const (
	// keyPrefix scopes commitment records in world state. Using a U+0000
	// separator (rather than ":" or "/") avoids collisions with policy
	// strings or hex characters and is CouchDB-safe.
	keyPrefix = "commitment\u0000"
)

// CommitmentKey returns the canonical world-state key for a commitment hash.
// The hash is lower-cased so callers don't need to remember the convention.
func CommitmentKey(c string) string {
	return keyPrefix + strings.ToLower(c)
}

// Get returns the commitment with hash c, or gcep.ErrCommitmentNotFound if
// no such record exists. Other errors (CouchDB unreachable, etc.) propagate.
func Get(ctx contractapi.TransactionContextInterface, c string) (*gcep.Commitment, error) {
	raw, err := ctx.GetStub().GetState(CommitmentKey(c))
	if err != nil {
		return nil, fmt.Errorf("getstate: %w", err)
	}
	if raw == nil {
		return nil, gcep.ErrCommitmentNotFound
	}
	return gcep.UnmarshalCommitment(raw)
}

// Put writes a commitment to world state. It does NOT enforce status
// transitions — callers must do that before calling Put.
func Put(ctx contractapi.TransactionContextInterface, cmt *gcep.Commitment) error {
	if cmt == nil {
		return errors.New("nil commitment")
	}
	raw, err := cmt.Marshal()
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := ctx.GetStub().PutState(CommitmentKey(cmt.C), raw); err != nil {
		return fmt.Errorf("putstate: %w", err)
	}
	return nil
}

// Exists returns true iff a commitment with hash c is present in world state.
// Cheaper than Get when the caller only needs presence.
func Exists(ctx contractapi.TransactionContextInterface, c string) (bool, error) {
	raw, err := ctx.GetStub().GetState(CommitmentKey(c))
	if err != nil {
		return false, fmt.Errorf("getstate: %w", err)
	}
	return raw != nil, nil
}
