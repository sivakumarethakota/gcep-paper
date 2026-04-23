// Command setup compiles the R_own circuit and runs Groth16 setup, writing
// the resulting proving key, verifying key, and R1CS to disk.
//
// USAGE
//
//	go run ./cmd/setup [-out ./keys]
//
// IMPORTANT — DEV ONLY. This command performs a single-party trusted setup,
// which means whoever runs it can forge proofs. For production the Groth16
// CRS must come from a multi-party Powers-of-Tau ceremony (see e.g. the
// Aztec/Semaphore transcripts, or gnark's `phase1`/`phase2` tools).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"

	"github.com/yourorg/gcep/circuits"
)

func main() {
	outDir := flag.String("out", "./keys", "output directory for r1cs.bin, proving.key, verifying.key")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outDir, err)
	}

	// 1. Compile
	log.Println("compiling R_own circuit…")
	t0 := time.Now()
	var circuit circuits.OwnCircuit
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	log.Printf("compiled in %s  (constraints=%d)", time.Since(t0), cs.GetNbConstraints())

	// 2. Groth16 setup
	log.Println("running Groth16 setup (DEV ONLY — single-party)…")
	t0 = time.Now()
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	log.Printf("setup complete in %s", time.Since(t0))

	// 3. Persist
	write := func(name string, writeTo func(*os.File) (int64, error)) int64 {
		path := filepath.Join(*outDir, name)
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("open %s: %v", path, err)
		}
		defer f.Close()
		n, err := writeTo(f)
		if err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		log.Printf("wrote %-16s  %d bytes", name, n)
		return n
	}

	write("r1cs.bin", func(f *os.File) (int64, error) { return cs.WriteTo(f) })
	write("proving.key", func(f *os.File) (int64, error) { return pk.WriteRawTo(f) })
	vkSize := write("verifying.key", func(f *os.File) (int64, error) { return vk.WriteRawTo(f) })

	fmt.Println()
	fmt.Println("=== summary ===")
	fmt.Printf("constraints       : %d\n", cs.GetNbConstraints())
	fmt.Printf("verifying key size: %d bytes  (this is what chaincode embeds)\n", vkSize)
	fmt.Printf("output directory  : %s\n", *outDir)
	fmt.Println()
	fmt.Println("Next step: run `go test ./...` to verify end-to-end Prove/Verify.")
}
