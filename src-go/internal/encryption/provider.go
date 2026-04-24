package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/jedisct1/go-aes-siv"
)

// EncryptionVersion represents the encryption version
type EncryptionVersion int

const (
	Version0 EncryptionVersion = 0
	Version2 EncryptionVersion = 2
	Version3 EncryptionVersion = 3
)

// EncryptionProvider interface for different encryption versions
type EncryptionProvider interface {
	// EncryptPath deterministically encrypts a path (for server storage)
	EncryptPath(path string) (string, error)
	// DecryptPath deterministically decrypts a path from server
	DecryptPath(encoded string) (string, error)
	// EncryptData encrypts file data (returns IV || ciphertext || tag)
	EncryptData(data []byte) ([]byte, error)
	// DecryptData decrypts file data
	DecryptData(data []byte) ([]byte, error)
}

// DeriveKey derives a 32-byte raw key from password and salt using scrypt
func DeriveKey(password, salt string) ([]byte, error) {
	normalizedPassword := norm.NFKC.String(password)
	normalizedSalt := norm.NFKC.String(salt)
	rawSalt := []byte(normalizedSalt) // UTF-8 encode the hex string

	rawKey, err := scrypt.Key([]byte(normalizedPassword), rawSalt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	return rawKey, nil
}

// ComputeKeyHash computes the key hash for server validation
func ComputeKeyHash(rawKey []byte, salt string, version EncryptionVersion) (string, error) {
	normalizedSalt := norm.NFKC.String(salt)
	rawSalt := []byte(normalizedSalt)

	switch version {
	case Version0:
		hash := sha256.Sum256(rawKey)
		return hex.EncodeToString(hash[:]), nil
	case Version2, Version3:
		// HKDF with info "ObsidianKeyHash"
		hkdfReader := hkdf.New(sha256.New, rawKey, rawSalt, []byte("ObsidianKeyHash"))
		keyHash := make([]byte, 32)
		if _, err := hkdfReader.Read(keyHash); err != nil {
			return "", fmt.Errorf("HKDF derivation failed: %w", err)
		}
		return hex.EncodeToString(keyHash), nil
	default:
		return "", fmt.Errorf("unsupported encryption version: %d", version)
	}
}

// NewEncryptionProvider creates the appropriate encryption provider
func NewEncryptionProvider(version EncryptionVersion, rawKey []byte, salt string) (EncryptionProvider, error) {
	switch version {
	case Version0:
		return newEncryptionV0(rawKey)
	case Version2, Version3:
		return newEncryptionV2V3(rawKey, salt, version)
	default:
		return nil, fmt.Errorf("unsupported encryption version: %d", version)
	}
}

// --- V0 Provider (legacy AES-GCM with deterministic IV) ---

type encryptionV0 struct {
	keyHash string
	gcm     cipher.AEAD
}

func newEncryptionV0(rawKey []byte) (*encryptionV0, error) {
	if len(rawKey) != 32 {
		return nil, fmt.Errorf("invalid encryption key length")
	}
	keyHash, _ := ComputeKeyHash(rawKey, "", Version0)
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &encryptionV0{keyHash: keyHash, gcm: gcm}, nil
}

func (e *encryptionV0) EncryptPath(path string) (string, error) {
	// Deterministic IV: SHA-256(path)[0..12]
	hash := sha256.Sum256([]byte(path))
	iv := hash[:12]
	plaintext := []byte(path)
	ciphertext := e.gcm.Seal(nil, iv, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

func (e *encryptionV0) DecryptPath(encoded string) (string, error) {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	iv := sha256.Sum256(data)
	plaintext, err := e.gcm.Open(nil, iv[:12], data, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (e *encryptionV0) EncryptData(data []byte) ([]byte, error) {
	iv := make([]byte, 12)
	return e.gcm.Seal(iv, iv, data, nil), nil
}

func (e *encryptionV0) DecryptData(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := data[:12]
	ciphertext := data[12:]
	return e.gcm.Open(nil, iv, ciphertext, nil)
}

// --- V2/V3 Provider (AES-SIV paths + AES-GCM content) ---

type encryptionV2V3 struct {
	keyHash string
	siv     *aessiv.AESSIV
	gcm     cipher.AEAD
}

func newEncryptionV2V3(rawKey []byte, salt string, version EncryptionVersion) (*encryptionV2V3, error) {
	if len(rawKey) != 32 {
		return nil, fmt.Errorf("invalid encryption key length")
	}

	// Compute key hash
	keyHash, err := ComputeKeyHash(rawKey, salt, version)
	if err != nil {
		return nil, err
	}

	// Import AES-SIV using derived key from HKDF
	// TypeScript uses TWO keys: "ObsidianAesSivEnc" and "ObsidianAesSivMac"
	normalizedSalt := norm.NFKC.String(salt)
	rawSalt := []byte(normalizedSalt)

	// Derive ENC key with info "ObsidianAesSivEnc"
	hkdfReader := hkdf.New(sha256.New, rawKey, rawSalt, []byte("ObsidianAesSivEnc"))
	sivEncKey := make([]byte, 32)
	if _, err := hkdfReader.Read(sivEncKey); err != nil {
		return nil, fmt.Errorf("failed to derive SIV ENC key: %w", err)
	}

	// Derive MAC key with info "ObsidianAesSivMac"
	hkdfReader = hkdf.New(sha256.New, rawKey, rawSalt, []byte("ObsidianAesSivMac"))
	sivMacKey := make([]byte, 32)
	if _, err := hkdfReader.Read(sivMacKey); err != nil {
		return nil, fmt.Errorf("failed to derive SIV MAC key: %w", err)
	}

	// Combine keys for go-aes-siv: MAC key first, then ENC key
	combinedKey := make([]byte, 64)
	copy(combinedKey[0:32], sivMacKey)
	copy(combinedKey[32:64], sivEncKey)

	siv, err := aessiv.New(combinedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES-SIV: %w", err)
	}

	// Derive AES-GCM content key (salt = empty!)
	hkdfReader = hkdf.New(sha256.New, rawKey, []byte{}, []byte("ObsidianAesGcm"))
	gcmKey := make([]byte, 32)
	if _, err := hkdfReader.Read(gcmKey); err != nil {
		return nil, fmt.Errorf("failed to derive GCM key: %w", err)
	}
	block, err := aes.NewCipher(gcmKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &encryptionV2V3{
		keyHash: keyHash,
		siv:     siv,
		gcm:     gcm,
	}, nil
}

func (e *encryptionV2V3) EncryptPath(path string) (string, error) {
	plaintext := []byte(path)
	ciphertext := e.siv.Seal(nil, nil, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

func (e *encryptionV2V3) DecryptPath(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return encoded, nil
	}
	plaintext, err := e.siv.Open(nil, nil, data, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (e *encryptionV2V3) EncryptData(data []byte) ([]byte, error) {
	iv := make([]byte, 12)
	return e.gcm.Seal(iv, iv, data, nil), nil
}

func (e *encryptionV2V3) DecryptData(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := data[:12]
	ciphertext := data[12:]
	return e.gcm.Open(nil, iv, ciphertext, nil)
}
