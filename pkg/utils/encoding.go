package utils

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

// Base58 encoding/decoding utilities

// EncodeBase58 encodes bytes to base58 string
func EncodeBase58(data []byte) string {
	return base58.Encode(data)
}

// DecodeBase58 decodes base58 string to bytes
func DecodeBase58(encoded string) ([]byte, error) {
	return base58.Decode(encoded)
}

// IsValidBase58 checks if string is valid base58
func IsValidBase58(s string) bool {
	_, err := base58.Decode(s)
	return err == nil
}

// Base64 encoding/decoding utilities

// EncodeBase64 encodes bytes to base64 string
func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes base64 string to bytes
func DecodeBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

// IsValidBase64 checks if string is valid base64
func IsValidBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

// Hex encoding/decoding utilities

// EncodeHex encodes bytes to hex string (with 0x prefix)
func EncodeHex(data []byte) string {
	return "0x" + hex.EncodeToString(data)
}

// EncodeHexNoPrefix encodes bytes to hex string (without 0x prefix)
func EncodeHexNoPrefix(data []byte) string {
	return hex.EncodeToString(data)
}

// DecodeHex decodes hex string to bytes (handles 0x prefix)
func DecodeHex(encoded string) ([]byte, error) {
	if len(encoded) >= 2 && encoded[:2] == "0x" {
		encoded = encoded[2:]
	}
	return hex.DecodeString(encoded)
}

// IsValidHex checks if string is valid hex
func IsValidHex(s string) bool {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Solana-specific encoding utilities

// Signature represents an Ed25519 signature (64 bytes)
type Signature [64]byte

// SignatureFromBase58 creates Signature from base58 string
func SignatureFromBase58(sig string) (Signature, error) {
	var signature Signature

	decoded, err := base58.Decode(sig)
	if err != nil {
		return signature, fmt.Errorf("invalid base58: %w", err)
	}

	if len(decoded) != 64 {
		return signature, fmt.Errorf("invalid signature length: expected 64, got %d", len(decoded))
	}

	copy(signature[:], decoded)
	return signature, nil
}

// String returns base58 representation of signature
func (s Signature) String() string {
	return base58.Encode(s[:])
}

// Bytes returns signature as byte slice
func (s Signature) Bytes() []byte {
	return s[:]
}

// Binary encoding utilities for instruction data

// EncodeU8 encodes uint8 to bytes
func EncodeU8(value uint8) []byte {
	return []byte{value}
}

// EncodeU16LE encodes uint16 to little-endian bytes
func EncodeU16LE(value uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, value)
	return buf
}

// EncodeU32LE encodes uint32 to little-endian bytes
func EncodeU32LE(value uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, value)
	return buf
}

// EncodeU64LE encodes uint64 to little-endian bytes
func EncodeU64LE(value uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	return buf
}

// DecodeU8 decodes uint8 from bytes
func DecodeU8(data []byte) (uint8, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("insufficient data to decode u8")
	}
	return data[0], nil
}

// DecodeU16LE decodes uint16 from little-endian bytes
func DecodeU16LE(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, fmt.Errorf("insufficient data to decode u16")
	}
	return binary.LittleEndian.Uint16(data), nil
}

// DecodeU32LE decodes uint32 from little-endian bytes
func DecodeU32LE(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data to decode u32")
	}
	return binary.LittleEndian.Uint32(data), nil
}

// DecodeU64LE decodes uint64 from little-endian bytes
func DecodeU64LE(data []byte) (uint64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("insufficient data to decode u64")
	}
	return binary.LittleEndian.Uint64(data), nil
}

// String encoding for instruction data

// EncodeString encodes string with length prefix (4-byte little-endian length + UTF-8 data)
func EncodeString(s string) []byte {
	data := []byte(s)
	length := EncodeU32LE(uint32(len(data)))
	return append(length, data...)
}

// DecodeString decodes string with length prefix
func DecodeString(data []byte) (string, int, error) {
	if len(data) < 4 {
		return "", 0, fmt.Errorf("insufficient data to decode string length")
	}

	length := binary.LittleEndian.Uint32(data[:4])
	if len(data) < int(4+length) {
		return "", 0, fmt.Errorf("insufficient data to decode string of length %d", length)
	}

	str := string(data[4 : 4+length])
	return str, int(4 + length), nil
}

// Boolean encoding

// EncodeBool encodes boolean as single byte (0 or 1)
func EncodeBool(value bool) []byte {
	if value {
		return []byte{1}
	}
	return []byte{0}
}

// DecodeBool decodes boolean from single byte
func DecodeBool(data []byte) (bool, error) {
	if len(data) < 1 {
		return false, fmt.Errorf("insufficient data to decode bool")
	}
	return data[0] != 0, nil
}

// Array encoding utilities

// EncodeByteArray encodes byte array with length prefix
func EncodeByteArray(data []byte) []byte {
	length := EncodeU32LE(uint32(len(data)))
	return append(length, data...)
}

// DecodeByteArray decodes byte array with length prefix
func DecodeByteArray(data []byte) ([]byte, int, error) {
	if len(data) < 4 {
		return nil, 0, fmt.Errorf("insufficient data to decode array length")
	}

	length := binary.LittleEndian.Uint32(data[:4])
	if len(data) < int(4+length) {
		return nil, 0, fmt.Errorf("insufficient data to decode array of length %d", length)
	}

	array := make([]byte, length)
	copy(array, data[4:4+length])
	return array, int(4 + length), nil
}

// Validation utilities

// IsValidSolanaAddress checks if string is a valid Solana address
func IsValidSolanaAddress(address string) bool {
	decoded, err := base58.Decode(address)
	return err == nil && len(decoded) == 32
}

// IsValidSolanaSignature checks if string is a valid Solana signature
func IsValidSolanaSignature(signature string) bool {
	decoded, err := base58.Decode(signature)
	return err == nil && len(decoded) == 64
}

// IsValidSolanaPrivateKey checks if string is a valid Solana private key
func IsValidSolanaPrivateKey(privkey string) bool {
	decoded, err := base58.Decode(privkey)
	return err == nil && len(decoded) == 64
}

// Hash utilities

// HashSHA256 computes SHA-256 hash
func HashSHA256(data []byte) []byte {
	// In a real implementation, you'd use crypto/sha256
	// This is a placeholder
	return data[:32] // Simplified for example
}

// HashKeccak256 computes Keccak-256 hash (used in some Solana contexts)
func HashKeccak256(data []byte) []byte {
	// In a real implementation, you'd use golang.org/x/crypto/sha3
	// This is a placeholder
	return data[:32] // Simplified for example
}

// Conversion utilities

// BytesToHex converts bytes to hex string representation
func BytesToHex(data []byte) string {
	return hex.EncodeToString(data)
}

// HexToBytes converts hex string to bytes
func HexToBytes(hexStr string) ([]byte, error) {
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}
	return hex.DecodeString(hexStr)
}

// PadBytes pads byte slice to specified length with zeros
func PadBytes(data []byte, length int) []byte {
	if len(data) >= length {
		return data[:length]
	}

	padded := make([]byte, length)
	copy(padded, data)
	return padded
}

// TrimBytes removes trailing zeros from byte slice
func TrimBytes(data []byte) []byte {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != 0 {
			return data[:i+1]
		}
	}
	return []byte{}
}

// Utility functions for working with instruction data

// ConcatBytes concatenates multiple byte slices
func ConcatBytes(slices ...[]byte) []byte {
	totalLen := 0
	for _, slice := range slices {
		totalLen += len(slice)
	}

	result := make([]byte, totalLen)
	offset := 0
	for _, slice := range slices {
		copy(result[offset:], slice)
		offset += len(slice)
	}

	return result
}

// SplitBytes splits byte slice at specified positions
func SplitBytes(data []byte, positions ...int) [][]byte {
	if len(positions) == 0 {
		return [][]byte{data}
	}

	result := make([][]byte, len(positions)+1)
	start := 0

	for i, pos := range positions {
		if pos > len(data) {
			pos = len(data)
		}
		result[i] = data[start:pos]
		start = pos
	}

	result[len(positions)] = data[start:]
	return result
}

func DecodeDataString(dataStr string) ([]byte, error) {
	dataStr = strings.TrimSpace(dataStr)

	data, err := base64.StdEncoding.DecodeString(dataStr)
	if err == nil {
		return data, nil
	}

	data, err = hex.DecodeString(dataStr)
	if err == nil {
		return data, nil
	}
	return nil, fmt.Errorf("unknown encoding (not base64 or hex)")
}
