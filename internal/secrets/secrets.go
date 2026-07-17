package secrets

import (
	cryptorand "crypto/rand"
	"encoding/base64"
)

func New() (string, error) {
	b := make([]byte, 32)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return "dpl_" + base64.RawURLEncoding.EncodeToString(b), nil
}
