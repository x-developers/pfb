package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/pkg/utils"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// ATAInfo represents Associated Token Account information
type ATAInfo struct {
	Address  []byte
	Mint     []byte
	Owner    []byte
	Exists   bool
	Balance  uint64
	Decimals uint8
}

// GetAssociatedTokenAccount derives the ATA address for a given mint and owner
func (w *Wallet) GetAssociatedTokenAccount(mint string) ([]byte, error) {
	mintBytes, err := base58.Decode(mint)
	if err != nil {
		return nil, fmt.Errorf("invalid mint address: %w", err)
	}

	return w.GetAssociatedTokenAccountForOwner(mintBytes, w.publicKey)
}

// GetAssociatedTokenAccountForOwner derives ATA address for specific owner
func (w *Wallet) GetAssociatedTokenAccountForOwner(mint, owner []byte) ([]byte, error) {
	// ATA address derivation using PDA (Program Derived Address)
	// Seeds: [owner, token_program_id, mint]

	seeds := [][]byte{
		owner,
		config.TokenProgramID,
		mint,
	}

	ata, _, err := w.findProgramAddress(seeds, config.AssociatedTokenProgramID)
	if err != nil {
		return nil, fmt.Errorf("failed to derive ATA address: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"mint":  base58.Encode(mint),
		"owner": base58.Encode(owner),
		"ata":   base58.Encode(ata),
	}).Debug("Derived ATA address")

	return ata, nil
}

// findProgramAddress finds a program derived address with bump seed
func (w *Wallet) findProgramAddress(seeds [][]byte, programID []byte) ([]byte, uint8, error) {
	// Try bump seeds from 255 down to 0
	for bump := uint8(255); ; bump-- {
		// Create seeds with bump
		seedsWithBump := make([][]byte, len(seeds)+1)
		copy(seedsWithBump, seeds)
		seedsWithBump[len(seeds)] = []byte{bump}

		// Try to create PDA
		pda, err := w.createProgramAddress(seedsWithBump, programID)
		if err == nil {
			return pda, bump, nil
		}

		if bump == 0 {
			break
		}
	}

	return nil, 0, fmt.Errorf("unable to find valid program address")
}

// createProgramAddress creates a program derived address
func (w *Wallet) createProgramAddress(seeds [][]byte, programID []byte) ([]byte, error) {
	const PDA_MARKER = "ProgramDerivedAddress"

	// Combine all seeds
	var combined []byte
	for _, seed := range seeds {
		if len(seed) > 32 {
			return nil, fmt.Errorf("seed too long: %d bytes", len(seed))
		}
		combined = append(combined, seed...)
	}

	// Add program ID
	combined = append(combined, programID...)

	// Add PDA marker
	combined = append(combined, []byte(PDA_MARKER)...)

	// Hash the combined data
	hash := sha256.Sum256(combined)

	// Check if the result is on the ed25519 curve (simplified check)
	// If the high bit is set, it's likely on the curve and invalid for PDA
	if hash[31]&0x80 != 0 {
		return nil, fmt.Errorf("address is on curve")
	}

	return hash[:], nil
}

// GetATAInfo gets detailed information about an ATA
func (w *Wallet) GetATAInfo(ctx context.Context, mint string) (*ATAInfo, error) {
	mintBytes, err := base58.Decode(mint)
	if err != nil {
		return nil, fmt.Errorf("invalid mint address: %w", err)
	}

	ata, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Get account info
	accountInfo, err := w.rpcClient.GetAccountInfo(ctx, base58.Encode(ata))
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA account info: %w", err)
	}

	info := &ATAInfo{
		Address: ata,
		Mint:    mintBytes,
		Owner:   w.publicKey,
		Exists:  accountInfo != nil,
	}

	if accountInfo != nil && len(accountInfo.Data) > 0 {
		// Parse token account data
		balance, decimals, err := w.parseTokenAccountData(accountInfo.Data)
		if err != nil {
			w.logger.WithError(err).Warn("Failed to parse token account data")
		} else {
			info.Balance = balance
			info.Decimals = decimals
		}
	}

	w.logger.WithFields(logrus.Fields{
		"mint":     mint,
		"ata":      base58.Encode(ata),
		"exists":   info.Exists,
		"balance":  info.Balance,
		"decimals": info.Decimals,
	}).Debug("Retrieved ATA info")

	return info, nil
}

// parseTokenAccountData parses token account data to extract balance and decimals
func (w *Wallet) parseTokenAccountData(data []string) (uint64, uint8, error) {
	if len(data) == 0 {
		return 0, 0, fmt.Errorf("empty token account data")
	}

	// Decode base64 data
	decoded, err := utils.DecodeBase64(data[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode token account data: %w", err)
	}

	// Token account structure:
	// 0-32: mint (32 bytes)
	// 32-64: owner (32 bytes)
	// 64-72: amount (8 bytes, little endian)
	// 72-76: delegate_option (4 bytes)
	// 76-77: state (1 byte)
	// 77-81: is_native_option (4 bytes)
	// 81-89: delegated_amount (8 bytes)
	// 89-93: close_authority_option (4 bytes)

	if len(decoded) < 72 {
		return 0, 0, fmt.Errorf("token account data too short: %d bytes", len(decoded))
	}

	// Extract balance (amount) from bytes 64-72
	balance := binary.LittleEndian.Uint64(decoded[64:72])

	// For pump.fun tokens, assume 6 decimals (standard)
	// In practice, you'd get this from the mint account
	decimals := uint8(6)

	return balance, decimals, nil
}

// CreateATAInstruction creates an instruction to create an ATA
func (w *Wallet) CreateATAInstruction(mint string) (CompiledInstruction, [][]byte, error) {
	mintBytes, err := base58.Decode(mint)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("invalid mint address: %w", err)
	}

	ata, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Account keys for ATA creation instruction
	accountKeys := [][]byte{
		w.publicKey,                     // Payer
		ata,                             // Associated token account
		w.publicKey,                     // Owner
		mintBytes,                       // Mint
		config.SystemProgramID,          // System program
		config.TokenProgramID,           // Token program
		config.AssociatedTokenProgramID, // Associated token program
	}

	// Create ATA instruction (no instruction data needed)
	instruction := CompiledInstruction{
		ProgramIdIndex: 6,                         // Associated token program index
		Accounts:       []uint8{0, 1, 2, 3, 4, 5}, // All accounts except program
		Data:           []byte{},                  // No data for create ATA instruction
	}

	w.logger.WithFields(logrus.Fields{
		"mint": mint,
		"ata":  base58.Encode(ata),
	}).Debug("Created ATA instruction")

	return instruction, accountKeys, nil
}

// CreateATAIfNeeded creates an ATA if it doesn't exist
func (w *Wallet) CreateATAIfNeeded(ctx context.Context, mint string) ([]byte, bool, error) {
	ata, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Check if ATA already exists
	info, err := w.GetATAInfo(ctx, mint)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get ATA info: %w", err)
	}

	if info.Exists {
		w.logger.WithField("ata", base58.Encode(ata)).Debug("ATA already exists")
		return ata, false, nil
	}

	// Create ATA instruction
	createInstruction, accountKeys, err := w.CreateATAInstruction(mint)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create ATA instruction: %w", err)
	}

	// Create transaction with proper account keys
	transaction := &SolanaTransaction{
		Signatures: make([][]byte, 1),
		Message: &SolanaMessage{
			Header: MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: uint8(len(accountKeys) - 1),
			},
			AccountKeys:  accountKeys,
			Instructions: []CompiledInstruction{createInstruction},
		},
	}

	// Get recent blockhash
	blockhash, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get blockhash: %w", err)
	}

	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		return nil, false, fmt.Errorf("failed to decode blockhash: %w", err)
	}

	copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

	// Sign and send transaction
	err = w.SignTransaction(transaction)
	if err != nil {
		return nil, false, fmt.Errorf("failed to sign transaction: %w", err)
	}

	signature, err := w.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return nil, false, fmt.Errorf("failed to send ATA creation transaction: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"mint":      mint,
		"ata":       base58.Encode(ata),
		"signature": signature,
	}).Info("Created ATA")

	return ata, true, nil
}

// GetATACreationCost estimates the cost to create an ATA
func (w *Wallet) GetATACreationCost() uint64 {
	// ATA creation cost is rent-exempt minimum balance for token account
	// Plus transaction fees
	rentExemptBalance := uint64(2039280) // ~0.002039 SOL for token account
	transactionFee := uint64(5000)       // Base transaction fee

	return rentExemptBalance + transactionFee
}

// ValidateTokenAmount validates token amount against account balance
func (w *Wallet) ValidateTokenAmount(ctx context.Context, mint string, amount uint64) error {
	info, err := w.GetATAInfo(ctx, mint)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if !info.Exists {
		return fmt.Errorf("token account does not exist for mint %s", mint)
	}

	if info.Balance < amount {
		return fmt.Errorf("insufficient token balance: have %d, need %d",
			info.Balance, amount)
	}

	return nil
}

// CreateTokenTransferInstruction creates an instruction to transfer tokens
func (w *Wallet) CreateTokenTransferInstruction(mint, recipient string, amount uint64) (CompiledInstruction, [][]byte, error) {
	// Get source ATA (wallet's ATA)
	sourceATA, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("failed to get source ATA: %w", err)
	}

	// Get destination ATA
	recipientBytes, err := base58.Decode(recipient)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("invalid recipient address: %w", err)
	}

	mintBytes, err := base58.Decode(mint)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("invalid mint address: %w", err)
	}

	destinationATA, err := w.GetAssociatedTokenAccountForOwner(mintBytes, recipientBytes)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("failed to get destination ATA: %w", err)
	}

	// Create transfer instruction data
	// Token transfer instruction discriminator is 3
	data := make([]byte, 9) // 1 byte discriminator + 8 bytes amount
	data[0] = 3             // Transfer instruction
	binary.LittleEndian.PutUint64(data[1:], amount)

	// Account keys for token transfer
	accountKeys := [][]byte{
		sourceATA,             // Source token account
		destinationATA,        // Destination token account
		w.publicKey,           // Owner of source account
		config.TokenProgramID, // Token program
	}

	instruction := CompiledInstruction{
		ProgramIdIndex: 3,                // Token program index
		Accounts:       []uint8{0, 1, 2}, // Source, destination, owner
		Data:           data,
	}

	w.logger.WithFields(logrus.Fields{
		"mint":            mint,
		"recipient":       recipient,
		"amount":          amount,
		"source_ata":      base58.Encode(sourceATA),
		"destination_ata": base58.Encode(destinationATA),
	}).Debug("Created token transfer instruction")

	return instruction, accountKeys, nil
}

// CreateCloseTokenAccountInstruction creates an instruction to close a token account
func (w *Wallet) CreateCloseTokenAccountInstruction(mint string) (CompiledInstruction, [][]byte, error) {
	// Get token account to close
	tokenAccount, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return CompiledInstruction{}, nil, fmt.Errorf("failed to get token account: %w", err)
	}

	// Create close account instruction data
	// Close account instruction discriminator is 9
	data := []byte{9}

	// Account keys for close account
	accountKeys := [][]byte{
		tokenAccount,          // Token account to close
		w.publicKey,           // Destination for remaining lamports
		w.publicKey,           // Owner of token account
		config.TokenProgramID, // Token program
	}

	instruction := CompiledInstruction{
		ProgramIdIndex: 3,                // Token program index
		Accounts:       []uint8{0, 1, 2}, // Account, destination, owner
		Data:           data,
	}

	w.logger.WithField("token_account", base58.Encode(tokenAccount)).Debug("Created close token account instruction")

	return instruction, accountKeys, nil
}

// GetTokenBalance gets token balance for a specific mint
func (w *Wallet) GetTokenBalance(ctx context.Context, mint string) (uint64, error) {
	info, err := w.GetATAInfo(ctx, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to get ATA info: %w", err)
	}

	if !info.Exists {
		return 0, nil // Account doesn't exist, balance is 0
	}

	return info.Balance, nil
}

// GetTokenBalanceFormatted gets token balance formatted with decimals
func (w *Wallet) GetTokenBalanceFormatted(ctx context.Context, mint string) (float64, error) {
	info, err := w.GetATAInfo(ctx, mint)
	if err != nil {
		return 0, fmt.Errorf("failed to get ATA info: %w", err)
	}

	if !info.Exists {
		return 0, nil
	}

	// Convert raw balance to decimal format
	decimals := info.Decimals
	if decimals == 0 {
		decimals = 6 // Default to 6 decimals
	}

	divisor := float64(1)
	for i := 0; i < int(decimals); i++ {
		divisor *= 10
	}

	return float64(info.Balance) / divisor, nil
}

// BatchCreateATAs creates multiple ATAs in a single transaction
func (w *Wallet) BatchCreateATAs(ctx context.Context, mints []string) (map[string][]byte, error) {
	if len(mints) == 0 {
		return make(map[string][]byte), nil
	}

	var instructions []CompiledInstruction
	var allAccountKeys [][]byte
	ataMap := make(map[string][]byte)
	accountOffset := 0

	// Create instructions for each mint
	for _, mint := range mints {
		// Check if ATA already exists
		info, err := w.GetATAInfo(ctx, mint)
		if err != nil {
			return nil, fmt.Errorf("failed to get ATA info for %s: %w", mint, err)
		}

		if info.Exists {
			ataMap[mint] = info.Address
			continue // Skip if already exists
		}

		// Create ATA instruction
		instruction, accountKeys, err := w.CreateATAInstruction(mint)
		if err != nil {
			return nil, fmt.Errorf("failed to create ATA instruction for %s: %w", mint, err)
		}

		// Adjust account indices for batch transaction
		for i := range instruction.Accounts {
			instruction.Accounts[i] += uint8(accountOffset)
		}
		instruction.ProgramIdIndex += uint8(accountOffset)

		instructions = append(instructions, instruction)
		allAccountKeys = append(allAccountKeys, accountKeys...)
		ataMap[mint] = info.Address
		accountOffset += len(accountKeys)
	}

	if len(instructions) == 0 {
		w.logger.Debug("All ATAs already exist")
		return ataMap, nil
	}

	// Create batch transaction
	transaction := &SolanaTransaction{
		Signatures: make([][]byte, 1),
		Message: &SolanaMessage{
			Header: MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: uint8(len(allAccountKeys) - 1),
			},
			AccountKeys:  allAccountKeys,
			Instructions: instructions,
		},
	}

	// Get recent blockhash
	blockhash, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blockhash: %w", err)
	}

	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode blockhash: %w", err)
	}

	copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

	// Sign and send transaction
	err = w.SignTransaction(transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to sign batch transaction: %w", err)
	}

	signature, err := w.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to send batch ATA creation transaction: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"mints":     len(mints),
		"created":   len(instructions),
		"signature": signature,
	}).Info("Batch created ATAs")

	return ataMap, nil
}

// IsATAInitialized checks if an ATA is initialized and ready to use
func (w *Wallet) IsATAInitialized(ctx context.Context, mint string) (bool, error) {
	info, err := w.GetATAInfo(ctx, mint)
	if err != nil {
		return false, fmt.Errorf("failed to get ATA info: %w", err)
	}

	return info.Exists, nil
}

// GetAllTokenBalances gets balances for all tokens owned by the wallet
func (w *Wallet) GetAllTokenBalances(ctx context.Context) (map[string]uint64, error) {
	// This would require querying all token accounts owned by the wallet
	// For now, we'll return an empty map as this requires more complex RPC calls
	// In practice, you'd use the getTokenAccountsByOwner RPC method
	balances := make(map[string]uint64)

	w.logger.Debug("Retrieved all token balances (placeholder implementation)")

	return balances, nil
}

// GetTokenAccountsByOwner gets all token accounts owned by the wallet for a specific mint
func (w *Wallet) GetTokenAccountsByOwner(ctx context.Context, mint string) ([]string, error) {
	// For simplicity, we'll return the single ATA
	// In practice, there could be multiple token accounts for the same mint
	ata, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA: %w", err)
	}

	accounts := []string{base58.Encode(ata)}

	w.logger.WithFields(logrus.Fields{
		"mint":     mint,
		"accounts": len(accounts),
	}).Debug("Retrieved token accounts by owner")

	return accounts, nil
}

// GetMintInfo gets information about a token mint
func (w *Wallet) GetMintInfo(ctx context.Context, mint string) (map[string]interface{}, error) {
	// Get mint account info
	accountInfo, err := w.rpcClient.GetAccountInfo(ctx, mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get mint account info: %w", err)
	}

	if accountInfo == nil {
		return nil, fmt.Errorf("mint account not found: %s", mint)
	}

	// Parse mint account data (simplified)
	// Mint account structure:
	// 0-4: mint_authority_option (4 bytes)
	// 4-36: mint_authority (32 bytes, if present)
	// 36-44: supply (8 bytes)
	// 44: decimals (1 byte)
	// 45: is_initialized (1 byte)
	// 46-50: freeze_authority_option (4 bytes)
	// 50-82: freeze_authority (32 bytes, if present)

	metadata := map[string]interface{}{
		"mint":     mint,
		"exists":   true,
		"decimals": 6, // Default assumption for pump.fun tokens
		"supply":   0, // Would need to parse mint account data properly
	}

	if len(accountInfo.Data) > 0 && len(accountInfo.Data[0]) > 44 {
		// Try to decode mint data
		decoded, err := utils.DecodeBase64(accountInfo.Data[0])
		if err == nil && len(decoded) > 44 {
			// Extract decimals (byte 44)
			decimals := decoded[44]
			metadata["decimals"] = decimals

			// Extract supply (bytes 36-44)
			if len(decoded) >= 44 {
				supply := binary.LittleEndian.Uint64(decoded[36:44])
				metadata["supply"] = supply
			}
		}
	}

	w.logger.WithField("mint", mint).Debug("Retrieved mint info")

	return metadata, nil
}

// CreateATACreationCostEstimate estimates the total cost of creating an ATA
func (w *Wallet) CreateATACreationCostEstimate(numATAs int) uint64 {
	costPerATA := w.GetATACreationCost()
	return uint64(numATAs) * costPerATA
}

// GetATAAddress returns the ATA address for a mint without checking if it exists
func (w *Wallet) GetATAAddress(mint string) (string, error) {
	ata, err := w.GetAssociatedTokenAccount(mint)
	if err != nil {
		return "", err
	}
	return base58.Encode(ata), nil
}
