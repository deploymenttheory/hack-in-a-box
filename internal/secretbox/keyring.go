// Package secretbox provides authenticated, context-bound encryption. Deployment
// key material stays outside PostgreSQL. Callers own authorization and auditing.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
)

var (
	ErrUnavailable   = errors.New("credential encryption unavailable")
	ErrConfiguration = errors.New("invalid credential keyring configuration")
	identifier       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type Keyring struct {
	active string
	keys   map[string]cipher.AEAD
}

// Parse accepts an empty configuration to disable secret operations. Errors
// deliberately never include configuration values or the underlying decoder.
func Parse(raw string) (*Keyring, error) {
	r := &Keyring{keys: map[string]cipher.AEAD{}}
	if raw == "" {
		return r, nil
	}
	var settings struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, ErrConfiguration
	}
	if !identifier.MatchString(settings.Active) || len(settings.Keys) == 0 ||
		len(settings.Keys) > 100 {
		return nil, ErrConfiguration
	}
	for id, encoded := range settings.Keys {
		if !identifier.MatchString(id) {
			return nil, ErrConfiguration
		}
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			return nil, ErrConfiguration
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, ErrConfiguration
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrConfiguration
		}
		r.keys[id] = aead
	}
	if r.keys[settings.Active] == nil {
		return nil, ErrConfiguration
	}
	r.active = settings.Active
	return r, nil
}

func (r *Keyring) Seal(value, binding []byte) (string, []byte, error) {
	if r == nil || r.keys[r.active] == nil {
		return "", nil, ErrUnavailable
	}
	// #nosec G407 -- NewGCMWithRandomNonce generates and prepends the nonce internally.
	return r.active, r.keys[r.active].Seal(nil, nil, value, binding), nil
}

func (r *Keyring) Open(id string, value, binding []byte) ([]byte, error) {
	if r == nil || r.keys[id] == nil {
		return nil, ErrUnavailable
	}
	plaintext, err := r.keys[id].Open(nil, nil, value, binding)
	if err != nil {
		return nil, ErrUnavailable
	}
	return plaintext, nil
}
