// Package secrets protects credentials that Socrates stores.
//
// The database is encrypted, but its key lives in the operating system's keychain, where a program running as the
// same user can sometimes reach it. A sealed secret adds a second lock that only the user's passphrase opens: a stolen
// database file, a backup, or malware that reads the keychain gets ciphertext, not the key.
//
// The cost of the passphrase hashing is stored inside every sealed value, so it can be raised later without breaking
// what was sealed earlier; the cost found in a stored value is bounded, so a crafted value cannot exhaust memory.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	sealedPrefix = "sealed:v2:"
	plainPrefix  = "plain:"
	defaultLogN  = 17 // scrypt N = 2^17 (128 MiB, a few tenths of a second per guess); r=8, p=1
	minLogN      = 10
	maxLogN      = 20 // 1 GiB: the most a stored value may ask for
	scryptR      = 8
	scryptP      = 1
	saltLen      = 16
	nonceLen     = 12
	tagLen       = 16
	// MinPassphraseLength is the shortest passphrase accepted. The sealed key sits in a file, so a thief can guess
	// offline; length is the only protection against that.
	MinPassphraseLength = 12
)

var (
	ErrWrongPassphrase = errors.New("wrong passphrase")
	ErrShortPass       = errors.New("the passphrase must be at least 12 characters")
)

// sealLogN is the cost used for new values. Tests lower it; nothing else changes it.
var sealLogN = defaultLogN

func deriveKey(pass string, salt []byte, logN int) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, 1<<logN, scryptR, scryptP, 32)
}

// header authenticates the version and the cost together with the secret.
func header(logN int) []byte { return append([]byte(sealedPrefix), byte(logN)) }

// Seal encrypts secret with a key derived from the passphrase (scrypt, AES-256-GCM) and returns a storable string.
func Seal(passphrase, secret string) (string, error) {
	if len([]rune(passphrase)) < MinPassphraseLength {
		return "", ErrShortPass
	}
	logN := sealLogN
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := deriveKey(passphrase, salt, logN)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	blob := []byte{byte(logN)}
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, gcm.Seal(nil, nonce, []byte(secret), header(logN))...)
	return sealedPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// Open reverses Seal. A wrong passphrase and a damaged value both give ErrWrongPassphrase.
func Open(passphrase, stored string) (string, error) {
	if !IsSealed(stored) {
		return "", errors.New("not a sealed value")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	if err != nil || len(raw) < 1+saltLen+nonceLen+tagLen {
		return "", ErrWrongPassphrase
	}
	logN := int(raw[0])
	if logN < minLogN || logN > maxLogN {
		return "", ErrWrongPassphrase
	}
	salt := raw[1 : 1+saltLen]
	nonce := raw[1+saltLen : 1+saltLen+nonceLen]
	ct := raw[1+saltLen+nonceLen:]
	key, err := deriveKey(passphrase, salt, logN)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, ct, header(logN))
	if err != nil {
		return "", ErrWrongPassphrase
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func IsSealed(stored string) bool { return strings.HasPrefix(stored, sealedPrefix) }

// Plain marks a value that is stored without a passphrase (still inside the encrypted database).
func Plain(secret string) string { return plainPrefix + secret }

// FromPlain returns the secret of a Plain value.
func FromPlain(stored string) (string, bool) {
	if strings.HasPrefix(stored, plainPrefix) {
		return strings.TrimPrefix(stored, plainPrefix), true
	}
	return "", false
}
