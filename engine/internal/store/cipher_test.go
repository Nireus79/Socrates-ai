package store

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, keySize) }

func TestFieldCipherRoundTripsWhateverIsStored(t *testing.T) {
	c, err := newFieldCipher(testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 1<<20)
	rand.Read(big)
	for name, plain := range map[string][]byte{"empty": {}, "text": []byte("What is courage?"), "unicode": []byte("Τι είναι η ανδρεία; ☉"), "binary": {0, 1, 2, 255}, "megabyte": big} {
		blob, err := c.seal("messages", "content", "row1", plain)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, err := c.open("messages", "content", "row1", blob)
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestACiphertextDoesNotShowItsPlaintextAndEveryOneIsDifferent(t *testing.T) {
	c, _ := newFieldCipher(testKey(1))
	plain := []byte("the same words every time")
	a, _ := c.seal("t", "c", "r", plain)
	b, _ := c.seal("t", "c", "r", plain)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same value must differ (random nonce)")
	}
	if bytes.Contains(a, []byte("same words")) {
		t.Fatal("the plaintext is visible in the ciphertext")
	}
}

func TestACiphertextCannotBeMovedToAnotherRowColumnOrTable(t *testing.T) {
	c, _ := newFieldCipher(testKey(1))
	blob, _ := c.seal("messages", "content", "row1", []byte("secret"))
	for _, tc := range [][3]string{{"messages", "content", "row2"}, {"messages", "title", "row1"}, {"conversations", "content", "row1"}} {
		if _, err := c.open(tc[0], tc[1], tc[2], blob); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("opened as %v: %v", tc, err)
		}
	}
}

func TestAnyChangeToACiphertextIsDetected(t *testing.T) {
	c, _ := newFieldCipher(testKey(1))
	blob, _ := c.seal("t", "c", "r", []byte("a value that matters"))
	for i := range blob {
		bad := append([]byte(nil), blob...)
		bad[i] ^= 0x01
		if _, err := c.open("t", "c", "r", bad); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("a flipped bit at byte %d went unnoticed: %v", i, err)
		}
	}
	for _, short := range [][]byte{nil, {}, {1}, blob[:5], blob[:len(blob)-1]} {
		if _, err := c.open("t", "c", "r", short); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("a truncated value was accepted: %v", err)
		}
	}
	wrongVersion := append([]byte(nil), blob...)
	wrongVersion[0] = 2
	if _, err := c.open("t", "c", "r", wrongVersion); !errors.Is(err, ErrCorrupt) {
		t.Fatal("an unknown version was accepted")
	}
}

func TestAnotherKeyCannotOpenAndTheKeyMustBe32Bytes(t *testing.T) {
	a, _ := newFieldCipher(testKey(1))
	b, _ := newFieldCipher(testKey(2))
	blob, _ := a.seal("t", "c", "r", []byte("secret"))
	if _, err := b.open("t", "c", "r", blob); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("another key opened it: %v", err)
	}
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := newFieldCipher(make([]byte, n)); err == nil {
			t.Fatalf("a key of %d bytes was accepted", n)
		}
	}
	if !strings.Contains(hkdfInfo, "v1") {
		t.Fatal("the key derivation must carry a version")
	}
}
