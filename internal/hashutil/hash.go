package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex returns the lowercase hex-encoded SHA-256 of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
