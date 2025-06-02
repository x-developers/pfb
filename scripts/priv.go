package main

import (
	"crypto/ed25519"
	"fmt"
	"github.com/mr-tron/base58"
	bip39 "github.com/tyler-smith/go-bip39"
)

// Solana seed derivation: see https://github.com/solana-labs/solana-keygen/blob/master/cli/src/keygen.rs
func deriveSolanaSeed(mnemonic, passphrase string) []byte {
	// Seed phrase -> seed (BIP39, 64 bytes)
	seed := bip39.NewSeed(mnemonic, passphrase)
	return seed
}

func main() {
	mnemonic := "digital reform tent oxygen club outer over envelope inner sick adapt oval"
	passphrase := "" // обычно пусто

	seed := deriveSolanaSeed(mnemonic, passphrase)

	// Solana: derive ed25519 keypair from seed[0:32]
	seed32 := seed[:32]
	privateKey := ed25519.NewKeyFromSeed(seed32)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// В Solana приватный ключ обычно = append(private, public) // 64 байта
	privateKey64 := append(privateKey.Seed(), publicKey...)

	// Base58
	privateKeyBase58 := base58.Encode(privateKey64)
	privateKey644, _ := base58.Decode(privateKeyBase58)

	fmt.Printf("Base58 Private Key (64 bytes): %s\n", privateKeyBase58)
	fmt.Printf("Decoded length: %d\n", len(privateKey644))
	fmt.Printf("Decoded length: %d\n", len(privateKey64))
}
