---
title: Encryption
---

# Encryption

## Overview

Obsidian Headless uses the same end-to-end encryption protocol as Obsidian Sync to protect file content and paths before they are transmitted to the server. The encryption key is derived from a user-provided password and a vault-specific salt; the server never sees the plaintext key.

Three encryption versions exist:

| Version | Status | Description |
|---------|--------|-------------|
| **V0** | Legacy | AES-GCM with deterministic IV for paths |
| **V2/V3** | Current | AES-SIV for paths, HKDF-derived sub-keys |

Versions 2 and 3 use the same algorithm. The version number is passed through to the server for compatibility tracking.

## Key Derivation

All encryption versions derive the master key from a user password and vault salt using **scrypt**:

```text
raw_key = scrypt(
  password = NFKC(user_password),
  salt     = NFKC(vault_salt),
  N        = 32768,
  r        = 8,
  p        = 1,
  dkLen    = 32
)
```

Both the password and salt are Unicode NFKC-normalised before hashing. The resulting `raw_key` is a 32-byte (256-bit) symmetric key.

## Version 0 (Legacy)

### Key Hash

The key hash is sent to the server for authentication:

```text
key_hash = hex( SHA-256( raw_key ) )
```

### Path Encryption

Paths are encrypted using **AES-256-GCM** with a deterministic IV derived from the path string:

```text
iv = SHA-256( UTF-8(path) )[0:12]
encrypted_path = hex( AES-GCM-Encrypt(key=raw_key, iv=iv, plaintext=UTF-8(path)) )
```

Because the IV is deterministic (derived from the plaintext), the same path always produces the same ciphertext. This allows the server to detect duplicate paths without knowing the plaintext.

::: warning
Deterministic IV sacrifices IND-CPA security for path deduplication. This is considered acceptable because file paths have low entropy in practice and path uniqueness is enforced by the filesystem.
:::

### Content Encryption

File content is encrypted using **AES-256-GCM** with a random 12-byte IV:

```text
iv = random(12)
ciphertext = AES-GCM-Encrypt(key=raw_key, iv=iv, plaintext=content)
wire_format = iv || ciphertext || auth_tag
```

The IV is prepended to the ciphertext for transmission.

::: tip
A random 12-byte IV is generated for each encryption via the system CSPRNG. The IV is prepended to the ciphertext so the decrypt side can recover it.
:::

### Content Hash

Content hashes are encrypted using the same deterministic path encryption scheme before being sent to the server.

## Version 2/3 (Current)

### Key Derivation (Extended)

From the scrypt-derived `raw_key`, three sub-keys are derived using **HKDF-SHA-256**:

```text
hkdf_base = HKDF-Import(raw_key, algorithm="HKDF")

// 1. Key hash (sent to server for authentication)
key_hash_key = HKDF-DeriveKey(
  base     = hkdf_base,
  salt     = UTF-8(vault_salt),
  info     = UTF-8("ObsidianKeyHash"),
  hash     = SHA-256,
  alg      = AES-CBC-256,
  extractable = true
)
key_hash = hex( ExportKey(key_hash_key) )

// 2. AES-SIV sub-keys for path encryption (two keys derived internally)
siv_enc_key = HKDF-DeriveKey(
  base     = hkdf_base,
  salt     = UTF-8(vault_salt),
  info     = UTF-8("ObsidianAesSivEnc"),
  hash     = SHA-256,
  alg      = AES-CTR-256
)

siv_mac_key = HKDF-DeriveKey(
  base     = hkdf_base,
  salt     = UTF-8(vault_salt),
  info     = UTF-8("ObsidianAesSivMac"),
  hash     = SHA-256,
  alg      = AES-CBC-256
)

// 3. AES-GCM key for content encryption
gcm_key = HKDF-DeriveKey(
  base     = hkdf_base,
  salt     = empty,
  info     = UTF-8("ObsidianAesGcm"),
  hash     = SHA-256,
  alg      = AES-GCM-256
)
```

### Path Encryption (AES-SIV)

Paths are encrypted using **AES-SIV** (RFC 5297), a deterministic authenticated encryption scheme:

```text
encrypted_path = hex( AES-SIV-Seal(key=siv_keys, plaintext=UTF-8(path)) )
```

AES-SIV produces a 16-byte synthetic IV (SIV tag) prepended to the ciphertext:

```text
output = SIV_tag (16 bytes) || ciphertext
```

The SIV tag serves as both the IV for AES-CTR and as an authentication tag.

::: tip
AES-SIV provides deterministic encryption without the security drawbacks of a fixed IV, because the synthetic IV is cryptographically bound to the plaintext via CMAC.
:::

#### AES-SIV Internals

The implementation follows RFC 5297:

1. **S2V** (String-to-Vector): Computes the SIV tag from the plaintext using CMAC (AES-CBC-MAC with NIST SP 800-38B sub-key derivation):
   - `D = CMAC(K, 0^128)` — CMAC of the zero block
   - If `len(plaintext) >= 16`: XOR D into the last 16 bytes, then CMAC the result
   - If `len(plaintext) < 16`: Pad with `10*0`, dbl(D), XOR, then CMAC
   - The `dbl()` operation is left-shift with carry and conditional XOR with `0x87`

2. **AES-CTR encryption**: Uses the SIV tag (with bits 31 and 63 cleared) as the counter block for AES-CTR mode encryption.

3. **Decryption**: Reverse the CTR encryption, recompute S2V, and verify the tag matches (constant-time comparison).

### Content Encryption (AES-GCM)

File content is encrypted with **AES-256-GCM** using the HKDF-derived `gcm_key`:

```text
iv = random(12)
ciphertext = AES-GCM-Encrypt(key=gcm_key, iv=iv, plaintext=content)
wire_format = iv || ciphertext || auth_tag
```

This is the same scheme as V0 content encryption, but with a different key.

### Content Hash

Content hashes are encrypted using AES-SIV (same as path encryption):

```text
encrypted_hash = hex( AES-SIV-Seal(key=siv_keys, plaintext=UTF-8(hash)) )
```

## Comparison Summary

| Aspect | V0 | V2/V3 |
|--------|-----|--------|
| Key derivation | scrypt | scrypt |
| Key hash | `SHA-256(raw_key)` | `HKDF(raw_key, salt, "ObsidianKeyHash")` |
| Path encryption | AES-GCM (deterministic IV) | AES-SIV (RFC 5297) |
| Content encryption | AES-GCM (random IV) | AES-GCM (HKDF-derived key, random IV) |
| Path IV source | `SHA-256(path)[0:12]` | Synthetic IV from CMAC |
| Deterministic? | Yes (paths only) | Yes (paths and hashes) |
| Auth tag size | 16 bytes (GCM) | 16 bytes (SIV + GCM) |

## HKDF Info Strings

| Purpose | Info String |
|---------|-------------|
| Key hash for server auth | `"ObsidianKeyHash"` |
| AES-SIV encryption sub-key | `"ObsidianAesSivEnc"` |
| AES-SIV MAC sub-key | `"ObsidianAesSivMac"` |
| AES-GCM content key | `"ObsidianAesGcm"` |
