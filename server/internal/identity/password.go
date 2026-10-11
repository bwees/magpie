package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLength counts characters, not bytes.
const MinPasswordLength = 12

// OWASP's argon2id baseline: 19 MiB, two passes, one lane.
const (
	argonMemoryKiB  = 19 * 1024
	argonPasses     = 2
	argonLanes      = 1
	argonKeyBytes   = 32
	argonSaltBytes  = 16
	argonHashFormat = "$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s"
	argonParamsForm = "m=%d,t=%d,p=%d"
	argonSeparator  = "$"
	argonHashParts  = 6 // "", "argon2id", version, params, salt, key
)

const argonAlgorithm = "argon2id"

var (
	ErrWeakPassword  = fmt.Errorf("identity: password must be at least %d characters", MinPasswordLength)
	errMalformedHash = errors.New("identity: malformed password hash")
	passwordEncoding = base64.RawStdEncoding
)

func checkPasswordStrength(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrWeakPassword
	}
	return nil
}

// hashPassword returns the PHC string for password: algorithm, parameters, salt and key.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonPasses, argonMemoryKiB, argonLanes, argonKeyBytes)
	return fmt.Sprintf(argonHashFormat, argon2.Version, argonMemoryKiB, argonPasses, argonLanes,
		passwordEncoding.EncodeToString(salt), passwordEncoding.EncodeToString(key)), nil
}

// passwordMatches re-derives the key with the parameters stored in encoded, so older hashes keep working
// if the defaults change.
func passwordMatches(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, argonSeparator)
	if len(parts) != argonHashParts || parts[1] != argonAlgorithm {
		return false, errMalformedHash
	}
	var memory, passes uint32
	var lanes uint8
	if _, err := fmt.Sscanf(parts[3], argonParamsForm, &memory, &passes, &lanes); err != nil {
		return false, errMalformedHash
	}
	salt, err := passwordEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := passwordEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, lanes, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
