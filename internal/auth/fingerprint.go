// Package auth maps SSH public keys to Shellbound player identities.
// A player's permanent identity is the SHA256 fingerprint of their public
// key; there are no passwords.
package auth

import (
	"errors"
	"regexp"

	gossh "golang.org/x/crypto/ssh"
)

// ErrNoKey is returned when a session presents no public key.
var ErrNoKey = errors.New("auth: no public key presented")

// usernameRE validates usernames: 3-16 chars of [a-zA-Z0-9_-].
var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,16}$`)

// Fingerprint returns the canonical SHA256 fingerprint (e.g.
// "SHA256:gQfH...") for an SSH public key. It is the stable identity key
// stored in the players table.
func Fingerprint(key gossh.PublicKey) (string, error) {
	if key == nil {
		return "", ErrNoKey
	}
	return gossh.FingerprintSHA256(key), nil
}

// ValidUsername reports whether name is an acceptable Shellbound username:
// 3-16 characters drawn from letters, digits, underscore and hyphen.
func ValidUsername(name string) bool {
	return usernameRE.MatchString(name)
}
