package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// Encryption constants matching the web app
	encContext      = "copyman-enc-v1"
	encIterations   = 100000
	encKeyLength    = 32 // 256 bits for AES-256
	aesGCMNonceSize = 12 // Standard nonce size for AES-GCM
)

// EncryptedData represents the structure of encrypted content
type EncryptedData struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	Salt       string `json:"salt"`
}

// deriveEncKey derives an encryption key from password using PBKDF2.
// This matches the deriveEncKey function in the web app.
// Salt format: "copyman-enc-v1:{createdAt}"
func deriveEncKey(password, createdAt string) []byte {
	salt := fmt.Sprintf("%s:%s", encContext, createdAt)
	return pbkdf2.Key([]byte(password), []byte(salt), encIterations, encKeyLength, sha256.New)
}

// encryptString encrypts plaintext using AES-GCM with the given key.
// Returns base64-encoded ciphertext, IV, and salt.
func encryptString(plaintext string, key []byte) (*EncryptedData, error) {
	// Generate random IV
	iv := make([]byte, aesGCMNonceSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Generate random salt (for compatibility with web app structure)
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), nil)

	return &EncryptedData{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		IV:         base64.StdEncoding.EncodeToString(iv),
		Salt:       base64.StdEncoding.EncodeToString(salt),
	}, nil
}

// decryptString decrypts ciphertext using AES-GCM with the given key.
func decryptString(encData *EncryptedData, key []byte) (string, error) {
	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encData.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(encData.IV)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (wrong key?): %w", err)
	}

	return string(plaintext), nil
}

// encryptFile encrypts file data using AES-GCM with the given key.
// Returns encrypted data, IV, and salt (all base64 encoded).
func encryptFile(fileData []byte, key []byte) (*EncryptedData, error) {
	// Generate random IV
	iv := make([]byte, aesGCMNonceSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Generate random salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, iv, fileData, nil)

	return &EncryptedData{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		IV:         base64.StdEncoding.EncodeToString(iv),
		Salt:       base64.StdEncoding.EncodeToString(salt),
	}, nil
}

// decryptFile decrypts file data using AES-GCM with the given key.
func decryptFile(encData *EncryptedData, key []byte) ([]byte, error) {
	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encData.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(encData.IV)
	if err != nil {
		return nil, fmt.Errorf("failed to decode IV: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key?): %w", err)
	}

	return plaintext, nil
}

// bytesToHex converts bytes to hex string
func bytesToHex(b []byte) string {
	return hex.EncodeToString(b)
}

// hexToBytes converts hex string to bytes
func hexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
