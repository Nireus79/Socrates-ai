package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// Everything the user writes or receives, and every credential, is encrypted field by field before it reaches the database file:
// AES-256-GCM with a key derived (HKDF-SHA256) from the database key held by the keystore. Each value gets a fresh random 96-bit nonce
// (safe for far more values than this application will ever write under one key), and is bound to the table, the column and the row
// it belongs to as additional authenticated data, so a ciphertext cannot be moved to another row or column, and any change to it is
// detected on reading.
//
// What stays readable in the file: identifiers, timestamps, the philosopher a conversation is with, the role of a message, and the
// length of each value (ciphertext length). The threat model says so in plain words.

const (
	ciphertextVersion byte = 1
	nonceSize              = 12
	keySize                = 32
	hkdfInfo               = "socrates/store/field-encryption/v1"
	aadPrefix              = "socrates/v1|"
)

var (
	// ErrWrongKey is returned when the database cannot be opened with the key it was given.
	ErrWrongKey = errors.New("the database key does not open this database")
	// ErrCorrupt is returned when a stored value fails authentication: it was changed, moved or damaged.
	ErrCorrupt = errors.New("a stored value failed its integrity check")
)

type fieldCipher struct{ aead cipher.AEAD }

func newFieldCipher(master []byte) (*fieldCipher, error) {
	if len(master) != keySize {
		return nil, fmt.Errorf("the database key must be %d bytes", keySize)
	}
	derived := make([]byte, keySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, master, nil, []byte(hkdfInfo)), derived); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &fieldCipher{aead: aead}, nil
}

func aad(table, column, rowID string) []byte {
	return []byte(aadPrefix + table + "|" + column + "|" + rowID)
}

// seal encrypts plaintext for one field of one row. The result is version | nonce | ciphertext+tag.
func (c *fieldCipher) seal(table, column, rowID string, plaintext []byte) ([]byte, error) {
	out := make([]byte, 1+nonceSize, 1+nonceSize+len(plaintext)+c.aead.Overhead())
	out[0] = ciphertextVersion
	if _, err := rand.Read(out[1 : 1+nonceSize]); err != nil {
		return nil, err
	}
	return c.aead.Seal(out, out[1:1+nonceSize], plaintext, aad(table, column, rowID)), nil
}

// open reverses seal. Anything that is not exactly what seal made for this field gives ErrCorrupt.
func (c *fieldCipher) open(table, column, rowID string, blob []byte) ([]byte, error) {
	if len(blob) < 1+nonceSize+c.aead.Overhead() || blob[0] != ciphertextVersion {
		return nil, ErrCorrupt
	}
	plain, err := c.aead.Open(nil, blob[1:1+nonceSize], blob[1+nonceSize:], aad(table, column, rowID))
	if err != nil {
		return nil, ErrCorrupt
	}
	return plain, nil
}
