# 1Password SDK CGO Workaround

## Problem

The `github.com/byteness/keyring` library (used for secure secret storage) imports `github.com/1password/onepassword-sdk-go` to support 1Password as a keyring backend.

However, the 1Password SDK intentionally breaks compilation when `CGO_ENABLED=0` on Darwin and Linux. The file `client_builder_no_cgo.go` contains a reference to a non-existent symbol:

```go
//go:build !cgo && (darwin || linux)

func WithDesktopAppIntegration(accountName string) ClientOption {
	var _ = ERROR_WithDesktopAppIntegration_requires_CGO_To_Cross_Compile_See_README_CGO_Section
	return nil
}
```

This causes a compile error:

```
undefined: ERROR_WithDesktopAppIntegration_requires_CGO_To_Cross_Compile_See_README_CGO_Section
```

Since we build with `CGO_ENABLED=0` for cross-platform releases (including Darwin and Windows), this blocks compilation entirely.

## Why Not Enable CGO?

Enabling CGO for cross-compilation requires platform-specific C toolchains (e.g., osxcross for macOS). This significantly complicates the build and release process. Since we do not use the 1Password keyring backend in practice, maintaining CGO toolchains is unnecessary overhead.

## Solution

We replace the 1Password SDK module with a locally patched copy via `go.mod`:

```go.mod
replace github.com/1password/onepassword-sdk-go => ./internal/1passwordstub
```

The `internal/1passwordstub/` directory contains a full copy of `github.com/1password/onepassword-sdk-go@v0.4.1-beta.1` with a single change: `client_builder_no_cgo.go` is patched to remove the intentional compile error and instead returns a functional `ClientOption` (matching the CGO variant's behavior).

### Patch Details

**Original (`client_builder_no_cgo.go`):**
```go
func WithDesktopAppIntegration(accountName string) ClientOption {
	var _ = ERROR_WithDesktopAppIntegration_requires_CGO_To_Cross_Compile_See_README_CGO_Section
	return nil
}
```

**Patched:**
```go
func WithDesktopAppIntegration(accountName string) ClientOption {
	return func(c *Client) error {
		c.config.AccountName = &accountName
		return nil
	}
}
```

All other SDK files remain unchanged. The 1Password keyring backend will not be functional at runtime when using the stub (it will return errors), but we do not rely on this backend.

## Impact

- **Builds:** Cross-compilation for all targets (Linux, Darwin, Windows) works without CGO.
- **Releases:** GoReleaser can produce binaries for all requested platforms.
- **Functionality:** The 1Password keyring backend is non-functional, but all other backends (macOS Keychain with CGO, Windows Credential Manager, Linux Secret Service, KWallet, file-based fallback) remain available where applicable.

## Updating the Stub

If `byteness/keyring` upgrades to a newer version of `github.com/1password/onepassword-sdk-go`:

1. Update `byteness/keyring` in `go.mod`.
2. Copy the new 1Password SDK version into `internal/1passwordstub/`.
3. Re-apply the patch to `client_builder_no_cgo.go`.
4. Verify cross-compilation with `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/ob-go`.

## Future Improvements

The ideal long-term fix is to update `github.com/byteness/keyring` to exclude the 1Password SDK import on platforms where CGO is disabled, or to make the 1Password backend opt-in via build tags. Until then, this workaround allows us to maintain a simple, CGO-free build pipeline.
