package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/logging"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/storage"
	syncpkg "github.com/Belphemur/obsidian-headless/src-go/internal/sync"
	"github.com/Belphemur/obsidian-headless/src-go/internal/util"
)

func addSyncCommands(root *cobra.Command, app *App) {
	root.AddCommand(
		newSyncListRemoteCommand(app),
		newSyncListLocalCommand(app),
		newSyncCreateRemoteCommand(app),
		newSyncSetupCommand(app),
		newSyncConfigCommand(app),
		newSyncStatusCommand(app),
		newSyncUnlinkCommand(app),
		newSyncRunCommand(app),
	)
}

func newSyncListRemoteCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync-list-remote",
		Short: "List remote vaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			vaults, err := app.client().ListVaults(cmd.Context(), token, 3)
			if err != nil {
				return err
			}
			for _, vault := range vaults {
				writeLines(app.stdout, fmt.Sprintf("%s\t%s\t%s", vault.ID, vault.Name, vault.Host))
			}
			return nil
		},
	}
}

func newSyncListLocalCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync-list-local",
		Short: "List locally configured sync vaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := configpkg.ListLocalVaults()
			if err != nil {
				return err
			}
			for _, id := range ids {
				cfg, err := configpkg.ReadSyncConfig(id)
				if err != nil || cfg == nil {
					continue
				}
				writeLines(app.stdout, fmt.Sprintf("%s\t%s\t%s", cfg.VaultID, cfg.VaultName, cfg.VaultPath))
			}
			return nil
		},
	}
}

func newSyncCreateRemoteCommand(app *App) *cobra.Command {
	var name, encryption, password, region string
	command := &cobra.Command{
		Use:   "sync-create-remote",
		Short: "Create a remote sync vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			version := 0
			key := ""
			salt := ""
			switch encryption {
			case "", "e2ee":
				if password == "" {
					return fmt.Errorf("--password is required for e2ee vaults")
				}
				version = 3
				salt, err = util.RandomHex(16)
				if err != nil {
					return err
				}
				key, err = util.DerivePasswordHash(password, salt)
				if err != nil {
					return err
				}
			case "standard":
				// no key material needed
			default:
				return fmt.Errorf("unknown encryption mode %q: must be 'e2ee' or 'standard'", encryption)
			}
			vault, err := app.client().CreateVault(cmd.Context(), token, name, key, salt, region, version)
			if err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Created vault %s (%s)", vault.Name, vault.ID))
			return nil
		},
	}
	command.Flags().StringVar(&name, "name", "", "vault name")
	command.Flags().StringVar(&encryption, "encryption", "e2ee", "standard or e2ee")
	command.Flags().StringVar(&password, "password", "", "encryption password")
	command.Flags().StringVar(&region, "region", "", "vault region")
	return command
}

func newSyncSetupCommand(app *App) *cobra.Command {
	var vaultSelector, localPath, password, deviceName, configDir, statePath string
	command := &cobra.Command{
		Use:   "sync-setup",
		Short: "Attach a local folder to a remote vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultSelector == "" {
				return fmt.Errorf("--vault is required")
			}
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			vaults, err := app.client().ListVaults(cmd.Context(), token, 3)
			if err != nil {
				return err
			}
			vault, err := resolveVault(vaults, vaultSelector)
			if err != nil {
				return err
			}
			if vault.Password != "" && password == "" {
				return fmt.Errorf("--password is required for encrypted vaults")
			}
			if err := configpkg.ValidateConfigDir(configDir); err != nil {
				return err
			}
			keyHash := ""
			if password != "" {
				keyHash, err = util.DerivePasswordHash(password, vault.Salt)
				if err != nil {
					return err
				}
			}
			if err := app.client().ValidateVaultAccess(cmd.Context(), token, vault.ID, keyHash, vault.Host, vault.EncryptionVersion); err != nil {
				return err
			}
			absPath, err := filepath.Abs(localPath)
			if err != nil {
				return err
			}
			cfg := model.SyncConfig{VaultID: vault.ID, VaultName: vault.Name, VaultPath: absPath, Host: vault.Host, EncryptionVersion: vault.EncryptionVersion, EncryptionSalt: vault.Salt, ConflictStrategy: "merge", DeviceName: deviceName, ConfigDir: configDir, StatePath: statePath}
			if cfg.DeviceName == "" {
				cfg.DeviceName = configpkg.DefaultDeviceName()
			}
			if cfg.ConfigDir == "" {
				cfg.ConfigDir = ".obsidian"
			}
			if err := configpkg.WriteSyncConfig(cfg); err != nil {
				return err
			}
			// Store the encryption key in the vault's secrets store so it is
			// never written to the plain-text config file.
			if password != "" {
				masterKey, mkErr := configpkg.LoadOrCreateMasterKey()
				if mkErr != nil {
					return mkErr
				}
				statePath, spErr := configpkg.StatePath(cfg.VaultID, cfg.StatePath)
				if spErr != nil {
					return spErr
				}
				store, stErr := storage.Open(statePath)
				if stErr != nil {
					return stErr
				}
				defer store.Close()
				if setErr := store.SetSecret("encryption_key", password, masterKey); setErr != nil {
					return setErr
				}
			}
			statePathValue, err := configpkg.StatePath(cfg.VaultID, cfg.StatePath)
			if err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Configured %s at %s", cfg.VaultName, absPath), fmt.Sprintf("State DB: %s", statePathValue))
			return nil
		},
	}
	command.Flags().StringVar(&vaultSelector, "vault", "", "vault id or name")
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	command.Flags().StringVar(&password, "password", "", "encryption password")
	command.Flags().StringVar(&deviceName, "device-name", "", "device name")
	command.Flags().StringVar(&configDir, "config-dir", ".obsidian", "config directory")
	command.Flags().StringVar(&statePath, "state-path", "", "custom state database path")
	return command
}

func newSyncConfigCommand(app *App) *cobra.Command {
	var localPath, mode, conflictStrategy, excludedFolders, fileTypes, configs, deviceName, configDir, statePath string
	command := &cobra.Command{
		Use:   "sync-config",
		Short: "View or update sync settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configpkg.FindSyncConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no sync config for %s", localPath)
			}
			changed := false
			if cmd.Flags().Changed("mode") {
				cfg.SyncMode = mode
				changed = true
			}
			if cmd.Flags().Changed("conflict-strategy") {
				cfg.ConflictStrategy = conflictStrategy
				changed = true
			}
			if cmd.Flags().Changed("excluded-folders") {
				cfg.IgnoreFolders = configpkg.ParseCSV(excludedFolders)
				changed = true
			}
			if cmd.Flags().Changed("file-types") {
				cfg.AllowTypes = configpkg.ParseCSV(fileTypes)
				if err := configpkg.ValidateChoices(cfg.AllowTypes, configpkg.ValidFileTypes, "file type"); err != nil {
					return err
				}
				changed = true
			}
			if cmd.Flags().Changed("configs") {
				cfg.AllowSpecialFiles = configpkg.ParseCSV(configs)
				if err := configpkg.ValidateChoices(cfg.AllowSpecialFiles, configpkg.ValidConfigCategories, "config category"); err != nil {
					return err
				}
				changed = true
			}
			if cmd.Flags().Changed("device-name") {
				cfg.DeviceName = deviceName
				changed = true
			}
			if cmd.Flags().Changed("config-dir") {
				if err := configpkg.ValidateConfigDir(configDir); err != nil {
					return err
				}
				cfg.ConfigDir = configDir
				changed = true
			}
			if cmd.Flags().Changed("state-path") {
				cfg.StatePath = statePath
				changed = true
			}
			if !changed {
				printSyncConfig(app, *cfg)
				return nil
			}
			return configpkg.WriteSyncConfig(*cfg)
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	command.Flags().StringVar(&mode, "mode", "", "bidirectional, pull, or mirror")
	command.Flags().StringVar(&conflictStrategy, "conflict-strategy", "merge", "merge or conflict")
	command.Flags().StringVar(&excludedFolders, "excluded-folders", "", "comma-separated folders")
	command.Flags().StringVar(&fileTypes, "file-types", "", "comma-separated file types")
	command.Flags().StringVar(&configs, "configs", "", "comma-separated config categories")
	command.Flags().StringVar(&deviceName, "device-name", "", "device name")
	command.Flags().StringVar(&configDir, "config-dir", ".obsidian", "config directory")
	command.Flags().StringVar(&statePath, "state-path", "", "custom state database path")
	return command
}

func newSyncStatusCommand(app *App) *cobra.Command {
	var localPath string
	command := &cobra.Command{
		Use:   "sync-status",
		Short: "Show sync configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configpkg.FindSyncConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no sync config for %s", localPath)
			}
			printSyncConfig(app, *cfg)
			return nil
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	return command
}

func newSyncUnlinkCommand(app *App) *cobra.Command {
	var localPath string
	command := &cobra.Command{
		Use:   "sync-unlink",
		Short: "Remove local sync configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configpkg.FindSyncConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no sync config for %s", localPath)
			}
			return configpkg.RemoveSyncConfig(cfg.VaultID)
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	return command
}

func newSyncRunCommand(app *App) *cobra.Command {
	var localPath string
	var continuous bool
	command := &cobra.Command{
		Use:   "sync",
		Short: "Run sync for a configured vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			cfg, err := configpkg.FindSyncConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no sync config for %s", localPath)
			}
			// Load the vault encryption key from the encrypted secrets store.
			if cfg.EncryptionVersion != 0 {
				masterKey, mkErr := configpkg.LoadOrCreateMasterKey()
				if mkErr != nil {
					return mkErr
				}
				statePath, spErr := configpkg.StatePath(cfg.VaultID, cfg.StatePath)
				if spErr != nil {
					return spErr
				}
				store, stErr := storage.Open(statePath)
				if stErr != nil {
					return stErr
				}
				defer store.Close()
				encKey, ekErr := store.GetSecret("encryption_key", masterKey)
				if ekErr != nil {
					return ekErr
				}
				cfg.EncryptionKey = encKey
			}
			logPath, err := configpkg.LogPath(cfg.VaultID)
			if err != nil {
				return err
			}
			logger, cleanup, err := logging.NewFileLogger(app.stderr, logPath)
			if err != nil {
				return err
			}
			defer cleanup()
			engine := syncpkg.NewEngine(*cfg, token, logger)
			if continuous {
				return engine.RunContinuous(cmd.Context())
			}
			return engine.RunOnce(cmd.Context())
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	command.Flags().BoolVar(&continuous, "continuous", false, "run continuously")
	return command
}

func resolveVault(vaults []model.Vault, selector string) (*model.Vault, error) {
	for _, vault := range vaults {
		if vault.ID == selector || vault.UID == selector || vault.Name == selector {
			copy := vault
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("vault %q not found", selector)
}

func printSyncConfig(app *App, cfg model.SyncConfig) {
	writeLines(app.stdout,
		fmt.Sprintf("Vault: %s (%s)", cfg.VaultName, cfg.VaultID),
		fmt.Sprintf("Location: %s", cfg.VaultPath),
		fmt.Sprintf("Host: %s", cfg.Host),
		fmt.Sprintf("Sync mode: %s", valueOrDefault(cfg.SyncMode, "bidirectional")),
		fmt.Sprintf("Conflict strategy: %s", valueOrDefault(cfg.ConflictStrategy, "merge")),
		fmt.Sprintf("Device name: %s", valueOrDefault(cfg.DeviceName, configpkg.DefaultDeviceName())),
		fmt.Sprintf("Config directory: %s", valueOrDefault(cfg.ConfigDir, ".obsidian")),
	)
}
