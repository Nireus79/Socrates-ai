package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

const pass = "correct horse battery"

// The tests use a cheap cost; TestProductionCostIsHigh checks what ships.
func TestMain(m *testing.M) {
	sealLogN = minLogN
	m.Run()
}

func TestSealOpenRoundTrip(t *testing.T) {
	secret := "sk-ant-api03-ABCDEFGH"
	sealed, err := Seal(pass, secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, secret) || !IsSealed(sealed) {
		t.Fatalf("sealed value must not contain the secret: %s", sealed)
	}
	got, err := Open(pass, sealed)
	if err != nil || got != secret {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

func TestWrongPassphraseAndTamperingAreRefused(t *testing.T) {
	sealed, _ := Seal(pass, "secret-key")
	if _, err := Open("wrong passphrase!", sealed); err != ErrWrongPassphrase {
		t.Fatalf("wrong passphrase: %v", err)
	}
	tampered := sealed[:len(sealed)-4] + "AAAA"
	if _, err := Open(pass, tampered); err != ErrWrongPassphrase {
		t.Fatalf("tampered value must not open: %v", err)
	}
	if _, err := Open(pass, sealedPrefix+"!!!not base64"); err != ErrWrongPassphrase {
		t.Fatalf("garbage must not open: %v", err)
	}
	if _, err := Open(pass, "plain:abc"); err == nil {
		t.Fatal("a value that is not sealed must be refused")
	}
}

func TestSealingIsRandomisedAndNeedsALongEnoughPassphrase(t *testing.T) {
	a, _ := Seal(pass, "same")
	b, _ := Seal(pass, "same")
	if a == b {
		t.Fatal("salt and nonce must differ every time")
	}
	for _, short := range []string{"short", "elevenchars", ""} {
		if _, err := Seal(short, "x"); err != ErrShortPass {
			t.Fatalf("passphrase %q: %v", short, err)
		}
	}
}

func TestTheCostIsStoredAndAuthenticated(t *testing.T) {
	sealed, _ := Seal(pass, "secret")
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, sealedPrefix))
	if int(raw[0]) != minLogN {
		t.Fatalf("the cost belongs in the value: %d", raw[0])
	}
	// Changing the stored cost must not open the value (it is part of what GCM authenticates).
	raw[0] = byte(minLogN + 1)
	forged := sealedPrefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := Open(pass, forged); err != ErrWrongPassphrase {
		t.Fatalf("a changed cost must be refused: %v", err)
	}
}

func TestACraftedCostCannotExhaustMemory(t *testing.T) {
	sealed, _ := Seal(pass, "secret")
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, sealedPrefix))
	for _, bad := range []byte{0, 5, byte(maxLogN + 1), 30, 255} {
		raw[0] = bad
		start := time.Now()
		_, err := Open(pass, sealedPrefix+base64.StdEncoding.EncodeToString(raw))
		if err != ErrWrongPassphrase {
			t.Fatalf("cost %d: %v", bad, err)
		}
		if time.Since(start) > time.Second {
			t.Fatalf("cost %d was not refused at once", bad)
		}
	}
}

func TestProductionCostIsHigh(t *testing.T) {
	if defaultLogN < 17 || defaultLogN > maxLogN {
		t.Fatalf("the shipped cost is %d: it must be at least 2^17 and within the accepted range", defaultLogN)
	}
	if MinPassphraseLength < 12 {
		t.Fatal("the passphrase minimum must be at least 12")
	}
}

func TestPlainMarker(t *testing.T) {
	v := Plain("abc")
	if got, ok := FromPlain(v); !ok || got != "abc" || IsSealed(v) {
		t.Fatalf("plain round trip failed: %q %v", got, ok)
	}
	if _, ok := FromPlain(sealedPrefix + "xxx"); ok {
		t.Fatal("a sealed value is not plain")
	}
}
