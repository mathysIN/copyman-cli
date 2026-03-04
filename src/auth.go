package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// PBKDF2 constants matching the web app
	pbkdf2Iterations = 100000
	keyLength        = 32 // 256 bits
	authContext      = "copyman-auth-v1"
)

// deriveAuthKey derives an authentication key from password using PBKDF2.
// This matches the deriveAuthKey function in the web app.
// Salt format: "copyman-auth-v1:{createdAt}"
func deriveAuthKey(password, createdAt string) string {
	salt := fmt.Sprintf("%s:%s", authContext, createdAt)
	derivedKey := pbkdf2.Key([]byte(password), []byte(salt), pbkdf2Iterations, keyLength, sha256.New)
	return hex.EncodeToString(derivedKey)
}
