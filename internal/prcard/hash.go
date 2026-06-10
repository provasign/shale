package prcard

import (
	"crypto/sha256"
	"encoding/hex"
)

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
