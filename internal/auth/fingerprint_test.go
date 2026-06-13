package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func genKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("wrap ssh key: %v", err)
	}
	return sshPub
}

func TestFingerprintFormat(t *testing.T) {
	key := genKey(t)
	fp, err := Fingerprint(key)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint %q does not start with SHA256:", fp)
	}
	if len(fp) < len("SHA256:")+40 {
		t.Errorf("fingerprint %q suspiciously short", fp)
	}
}

func TestFingerprintStable(t *testing.T) {
	key := genKey(t)
	a, err := Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("fingerprint unstable: %q vs %q", a, b)
	}
}

func TestFingerprintDistinct(t *testing.T) {
	a, err := Fingerprint(genKey(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fingerprint(genKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two distinct keys produced the same fingerprint")
	}
}

func TestFingerprintNilKey(t *testing.T) {
	if _, err := Fingerprint(nil); err == nil {
		t.Error("expected error for nil key")
	}
}

func TestValidUsername(t *testing.T) {
	valid := []string{"abc", "Player_1", "a-b-c", "ABCDEFGHIJKLMNOP", "x_y", "123"}
	for _, v := range valid {
		if !ValidUsername(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	invalid := []string{"", "ab", "ABCDEFGHIJKLMNOPQ", "has space", "emoji✦", "dot.name", "slash/name"}
	for _, v := range invalid {
		if ValidUsername(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}
