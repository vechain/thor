package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/v2/thor"
)

// VRFSetup provides utilities for validators to set up their VRF keys
type VRFSetup struct {
}

// NewVRFSetup creates a new VRF setup utility
func NewVRFSetup() *VRFSetup {
	return &VRFSetup{}
}

// GeneratePublicKeyFromPrivateKey generates a public key from a private key
func (vs *VRFSetup) GeneratePublicKeyFromPrivateKey(privateKeyHex string) (string, error) {
	// Remove 0x prefix if present
	if len(privateKeyHex) >= 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	// Decode private key
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key format: %w", err)
	}

	// Parse private key
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	// Get public key
	publicKey := privateKey.Public().(*ecdsa.PublicKey)

	// Compress public key (33 bytes)
	compressedPublicKey := crypto.CompressPubkey(publicKey)

	return hex.EncodeToString(compressedPublicKey), nil
}

// RegisterPublicKey registers a validator's public key in the Staker contract
func (vs *VRFSetup) RegisterPublicKey(validatorAddress thor.Address, publicKeyHex string, privateKeyHex string) error {
	// Remove 0x prefix if present
	if len(publicKeyHex) >= 2 && publicKeyHex[:2] == "0x" {
		publicKeyHex = publicKeyHex[2:]
	}

	// Decode public key
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key format: %w", err)
	}

	// Validate public key length
	if len(publicKeyBytes) != 33 && len(publicKeyBytes) != 65 {
		return fmt.Errorf("invalid public key length: %d (expected 33 or 65)", len(publicKeyBytes))
	}

	// TODO: Implement the actual contract call to register the public key
	// This would involve creating a transaction that calls the Staker contract's
	// UpdateValidatorPublicKey method

	fmt.Printf("Public key registration for validator %s:\n", validatorAddress)
	fmt.Printf("Public key: %s\n", publicKeyHex)
	fmt.Printf("Length: %d bytes\n", len(publicKeyBytes))
	fmt.Printf("Note: This is a placeholder. Implement actual contract call.\n")

	return nil
}

// VerifyPublicKey verifies that a public key is registered for a validator
func (vs *VRFSetup) VerifyPublicKey(validatorAddress thor.Address) error {
	// TODO: Implement verification by calling the Staker contract's
	// GetValidatorPublicKey method

	fmt.Printf("Verifying public key for validator %s...\n", validatorAddress)
	fmt.Printf("Note: This is a placeholder. Implement actual contract call.\n")

	return nil
}

// PrintSetupInstructions prints setup instructions for validators
func (vs *VRFSetup) PrintSetupInstructions() {
	fmt.Println("=== VRF Validator Setup Instructions ===")
	fmt.Println()
	fmt.Println("1. Generate your public key from your private key:")
	fmt.Println("   thor vrf generate-public-key --private-key YOUR_PRIVATE_KEY")
	fmt.Println()
	fmt.Println("2. Register your public key in the Staker contract:")
	fmt.Println("   thor vrf register --validator YOUR_VALIDATOR_ADDRESS --public-key YOUR_PUBLIC_KEY --private-key YOUR_PRIVATE_KEY")
	fmt.Println()
	fmt.Println("3. Verify your public key is registered:")
	fmt.Println("   thor vrf verify --validator YOUR_VALIDATOR_ADDRESS")
	fmt.Println()
	fmt.Println("Note: Your private key is used to sign the transaction, not for VRF generation.")
	fmt.Println("The VRF system uses the same key pair as your validator's block signing key.")
}

// RunVRFSetup runs the VRF setup based on command line arguments
func RunVRFSetup(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: thor vrf <command> [options]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  generate-public-key --private-key <key>  Generate public key from private key")
		fmt.Println("  register --validator <addr> --public-key <key> --private-key <key>  Register public key")
		fmt.Println("  verify --validator <addr>  Verify public key registration")
		fmt.Println("  help  Show setup instructions")
		return nil
	}

	command := args[0]

	switch command {
	case "generate-public-key":
		return runGeneratePublicKey(args[1:])
	case "register":
		return runRegister(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "help":
		vs := &VRFSetup{}
		vs.PrintSetupInstructions()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func runGeneratePublicKey(args []string) error {
	var privateKeyHex string

	for i := 0; i < len(args); i++ {
		if args[i] == "--private-key" && i+1 < len(args) {
			privateKeyHex = args[i+1]
			break
		}
	}

	if privateKeyHex == "" {
		return fmt.Errorf("--private-key is required")
	}

	vs := &VRFSetup{}
	publicKeyHex, err := vs.GeneratePublicKeyFromPrivateKey(privateKeyHex)
	if err != nil {
		return err
	}

	fmt.Printf("Public key: %s\n", publicKeyHex)
	return nil
}

func runRegister(args []string) error {
	var validatorAddress, publicKeyHex, privateKeyHex string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--validator":
			if i+1 < len(args) {
				validatorAddress = args[i+1]
			}
		case "--public-key":
			if i+1 < len(args) {
				publicKeyHex = args[i+1]
			}
		case "--private-key":
			if i+1 < len(args) {
				privateKeyHex = args[i+1]
			}
		}
	}

	if validatorAddress == "" || publicKeyHex == "" || privateKeyHex == "" {
		return fmt.Errorf("--validator, --public-key, and --private-key are required")
	}

	validatorAddr, err := thor.ParseAddress(validatorAddress)
	if err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}

	vs := &VRFSetup{}
	return vs.RegisterPublicKey(validatorAddr, publicKeyHex, privateKeyHex)
}

func runVerify(args []string) error {
	var validatorAddress string

	for i := 0; i < len(args); i++ {
		if args[i] == "--validator" && i+1 < len(args) {
			validatorAddress = args[i+1]
			break
		}
	}

	if validatorAddress == "" {
		return fmt.Errorf("--validator is required")
	}

	validatorAddr, err := thor.ParseAddress(validatorAddress)
	if err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}

	vs := &VRFSetup{}
	return vs.VerifyPublicKey(validatorAddr)
}
