// Command gcep is the chaincode binary. Fabric's lifecycle invokes it with
// no arguments; the contractapi runtime takes over and dispatches to the
// SmartContract methods registered below.
package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"

	gcep "github.com/yourorg/gcep/chaincode"
)

func main() {
	cc, err := contractapi.NewChaincode(&gcep.SmartContract{})
	if err != nil {
		log.Fatalf("create chaincode: %v", err)
	}
	if err := cc.Start(); err != nil {
		log.Fatalf("start chaincode: %v", err)
	}
}
