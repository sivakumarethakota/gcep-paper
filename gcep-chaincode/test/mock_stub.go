// Package test provides a minimal in-memory Fabric stub. It implements just
// enough of `shim.ChaincodeStubInterface` and `cid.ClientIdentity` for the
// GCEP contract tests; nothing more.
//
// If you need richer behaviour (range queries, history queries, private
// data) bring in `hyperledger/fabric-chaincode-go/shim/shimtest` instead.
package test

import (
	"errors"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MockContext implements contractapi.TransactionContextInterface in-memory.
type MockContext struct {
	contractapi.TransactionContext
	stub *MockStub
	cid  *MockClientIdentity
}

// NewMockContext returns a context with a fresh empty world state and a
// default identity ("HospitalMSP/x509::CN=alice").
func NewMockContext() *MockContext {
	c := &MockContext{
		stub: NewMockStub(),
		cid:  NewMockClientIdentity("HospitalMSP", "x509::CN=alice::CN=ca"),
	}
	c.SetStub(c.stub)
	c.SetClientIdentity(c.cid)
	return c
}

// WithIdentity returns a context whose submitter is overridden. Useful when
// a test needs to assert that committee members can call FinaliseErase.
func (c *MockContext) WithIdentity(mspid, id string) *MockContext {
	c.cid = NewMockClientIdentity(mspid, id)
	c.SetClientIdentity(c.cid)
	return c
}

// Events returns every event SetEvent'd on this context, in order.
func (c *MockContext) Events() []MockEvent {
	return c.stub.events
}

// -----------------------------------------------------------------------------
// MockStub — minimal in-memory Fabric stub
// -----------------------------------------------------------------------------

type MockEvent struct {
	Name    string
	Payload []byte
}

type MockStub struct {
	shim.ChaincodeStubInterface
	state  map[string][]byte
	events []MockEvent
	txID   string
}

func NewMockStub() *MockStub {
	return &MockStub{
		state: map[string][]byte{},
		txID:  "mock-tx-" + time.Now().Format("150405.000000000"),
	}
}

func (s *MockStub) GetState(key string) ([]byte, error) {
	v, ok := s.state[key]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (s *MockStub) PutState(key string, value []byte) error {
	if key == "" {
		return errors.New("empty key")
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	s.state[key] = cp
	return nil
}

func (s *MockStub) DelState(key string) error {
	delete(s.state, key)
	return nil
}

func (s *MockStub) SetEvent(name string, payload []byte) error {
	s.events = append(s.events, MockEvent{Name: name, Payload: append([]byte(nil), payload...)})
	return nil
}

func (s *MockStub) GetTxID() string { return s.txID }

func (s *MockStub) GetTxTimestamp() (*timestamppb.Timestamp, error) {
	return timestamppb.New(time.Now()), nil
}

// Unused stub methods that contractapi may probe — return zero values rather
// than panicking, so we don't have to maintain a long whitelist.
func (s *MockStub) GetFunctionAndParameters() (string, []string)    { return "", nil }
func (s *MockStub) GetArgs() [][]byte                               { return nil }
func (s *MockStub) GetStringArgs() []string                         { return nil }
func (s *MockStub) GetCreator() ([]byte, error)                     { return nil, nil }
func (s *MockStub) InvokeChaincode(string, [][]byte, string) pb.Response {
	return pb.Response{Status: 200}
}

// -----------------------------------------------------------------------------
// MockClientIdentity
// -----------------------------------------------------------------------------

type MockClientIdentity struct {
	mspid string
	id    string
}

func NewMockClientIdentity(mspid, id string) *MockClientIdentity {
	return &MockClientIdentity{mspid: mspid, id: id}
}

func (c *MockClientIdentity) GetMSPID() (string, error)             { return c.mspid, nil }
func (c *MockClientIdentity) GetID() (string, error)                { return c.id, nil }
func (c *MockClientIdentity) GetX509Certificate() (any, error)      { return nil, nil }
func (c *MockClientIdentity) GetAttributeValue(string) (string, bool, error) {
	return "", false, nil
}
func (c *MockClientIdentity) AssertAttributeValue(string, string) error { return nil }
