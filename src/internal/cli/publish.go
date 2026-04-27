package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	publishpkg "github.com/Belphemur/obsidian-headless/src-go/internal/publish"
)

func addPublishCommands(root *cobra.Command, app *App) {
	root.AddCommand(
		newPublishListSitesCommand(app),
		newPublishCreateSiteCommand(app),
		newPublishSetupCommand(app),
		newPublishConfigCommand(app),
		newPublishUnlinkCommand(app),
		newPublishRunCommand(app),
	)
}

func newPublishListSitesCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "publish-list-sites",
		Short: "List publish sites",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			sites, err := app.client().ListPublishSites(cmd.Context(), token)
			if err != nil {
				return err
			}
			for _, site := range sites {
				writeLines(app.stdout, fmt.Sprintf("%s\t%s\t%s", site.ID, site.Slug, site.Host))
			}
			return nil
		},
	}
}

func newPublishCreateSiteCommand(app *App) *cobra.Command {
	var slug string
	command := &cobra.Command{
		Use:   "publish-create-site",
		Short: "Create a publish site",
		RunE: func(cmd *cobra.Command, args []string) error {
			if slug == "" {
				return fmt.Errorf("--slug is required")
			}
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			site, err := app.client().CreatePublishSite(cmd.Context(), token)
			if err != nil {
				return err
			}
			if err := app.client().SetPublishSlug(cmd.Context(), token, site.ID, site.Host, slug); err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Created publish site %s (%s)", slug, site.ID))
			return nil
		},
	}
	command.Flags().StringVar(&slug, "slug", "", "site slug")
	return command
}

func newPublishSetupCommand(app *App) *cobra.Command {
	var selector, localPath string
	command := &cobra.Command{
		Use:   "publish-setup",
		Short: "Attach a vault to a publish site",
		RunE: func(cmd *cobra.Command, args []string) error {
			if selector == "" {
				return fmt.Errorf("--site is required")
			}
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			sites, err := app.client().ListPublishSites(cmd.Context(), token)
			if err != nil {
				return err
			}
			site, err := resolveSite(sites, selector)
			if err != nil {
				return err
			}
			absPath, err := filepath.Abs(localPath)
			if err != nil {
				return err
			}
			cfg := model.PublishConfig{SiteID: site.ID, Host: site.Host, VaultPath: absPath}
			if err := configpkg.WritePublishConfig(cfg); err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Configured publish site %s for %s", valueOrDefault(site.Slug, site.ID), absPath))
			return nil
		},
	}
	command.Flags().StringVar(&selector, "site", "", "site id or slug")
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	return command
}

func newPublishConfigCommand(app *App) *cobra.Command {
	var localPath, includes, excludes string
	command := &cobra.Command{
		Use:   "publish-config",
		Short: "View or update publish settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configpkg.FindPublishConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no publish config for %s", localPath)
			}
			changed := false
			if cmd.Flags().Changed("includes") {
				cfg.Includes = configpkg.ParseCSV(includes)
				changed = true
			}
			if cmd.Flags().Changed("excludes") {
				cfg.Excludes = configpkg.ParseCSV(excludes)
				changed = true
			}
			if !changed {
				writeLines(app.stdout,
					fmt.Sprintf("Site: %s", cfg.SiteID),
					fmt.Sprintf("Location: %s", cfg.VaultPath),
					fmt.Sprintf("Includes: %v", cfg.Includes),
					fmt.Sprintf("Excludes: %v", cfg.Excludes),
				)
				return nil
			}
			return configpkg.WritePublishConfig(*cfg)
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	command.Flags().StringVar(&includes, "includes", "", "comma-separated include patterns")
	command.Flags().StringVar(&excludes, "excludes", "", "comma-separated exclude patterns")
	return command
}

func newPublishUnlinkCommand(app *App) *cobra.Command {
	var localPath string
	command := &cobra.Command{
		Use:   "publish-unlink",
		Short: "Remove local publish configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configpkg.FindPublishConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no publish config for %s", localPath)
			}
			return configpkg.RemovePublishConfig(cfg.SiteID)
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	return command
}

func newPublishRunCommand(app *App) *cobra.Command {
	var localPath string
	var dryRun, yes, all bool
	command := &cobra.Command{
		Use:   "publish",
		Short: "Publish vault changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := app.requireToken()
			if err != nil {
				return err
			}
			cfg, err := configpkg.FindPublishConfigByPath(localPath)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf("no publish config for %s", localPath)
			}
			engine := publishpkg.NewEngine(app.client(), *cfg, token)
			result, err := engine.Run(cmd.Context(), dryRun, yes, all)
			if err != nil {
				return err
			}
			for _, path := range result.Uploads {
				writeLines(app.stdout, "upload\t"+path)
			}
			for _, path := range result.Deletes {
				writeLines(app.stdout, "delete\t"+path)
			}
			return nil
		},
	}
	command.Flags().StringVar(&localPath, "path", ".", "local vault path")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without publishing")
	command.Flags().BoolVar(&yes, "yes", false, "apply changes without confirmation")
	command.Flags().BoolVar(&all, "all", false, "publish untagged files too")
	return command
}

func resolveSite(sites []model.PublishSite, selector string) (*model.PublishSite, error) {
	for _, site := range sites {
		if site.ID == selector || site.Slug == selector {
			copy := site
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("site %q not found", selector)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
