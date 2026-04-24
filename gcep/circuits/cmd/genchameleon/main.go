// Command genchameleon generates a fresh chameleon-hash keypair on BN254.
//
// Trapdoor x is sampled uniformly from Z_q; public key y = g^x in G1.
// The trapdoor MUST be kept secret (in production: split via Shamir
// across the committee). The public key is embedded in chaincode.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

func main() {
	outDir := flag.String("out", "./keys", "output directory")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	mod := fr.Modulus()
	x, err := rand.Int(rand.Reader, mod)
	if err != nil {
		log.Fatalf("rand: %v", err)
	}

	_, _, g, _ := bn254.Generators()
	var yJac bn254.G1Jac
	yJac.FromAffine(&g).ScalarMultiplication(&yJac, x)
	var y bn254.G1Affine
	y.FromJacobian(&yJac)

	xHex := hex.EncodeToString(x.Bytes())
	yHex := hex.EncodeToString(y.Marshal())

	if err := os.WriteFile(filepath.Join(*outDir, "trapdoor.hex"), []byte(xHex), 0o600); err != nil {
		log.Fatalf("write trapdoor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "pubkey.hex"), []byte(yHex), 0o644); err != nil {
		log.Fatalf("write pubkey: %v", err)
	}

	fmt.Printf("trapdoor (KEEP SECRET) → %s/trapdoor.hex   (%d hex chars)\n", *outDir, len(xHex))
	fmt.Printf("pubkey   (embed in cc) → %s/pubkey.hex     (%d hex chars)\n", *outDir, len(yHex))
	fmt.Println()
	fmt.Println("Public key (first 80 chars):", yHex[:80], "…")
}
