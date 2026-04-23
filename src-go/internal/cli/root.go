package cli

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type rootCommand struct {
	*cobra.Command
}

func (r *rootCommand) ExecuteContext(ctx context.Context) error {
	r.SetContext(ctx)
	return r.Execute()
}

func newRootCommand(app *App) *rootCommand {
	viper.SetDefault("api-base", "https://api.obsidian.md")
	viper.SetDefault("timeout", 30)
	viper.SetEnvPrefix("OBSIDIAN")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	root := &cobra.Command{
		Use:           "ob",
		Short:         "Go implementation of the Obsidian headless client",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.stdin)
	root.SetOut(app.stdout)
	root.SetErr(app.stderr)
	root.PersistentFlags().String("api-base", viper.GetString("api-base"), "Obsidian API base URL")
	root.PersistentFlags().Int("timeout", viper.GetInt("timeout"), "HTTP timeout in seconds")
	_ = viper.BindPFlag("api-base", root.PersistentFlags().Lookup("api-base"))
	_ = viper.BindPFlag("timeout", root.PersistentFlags().Lookup("timeout"))
	root.AddCommand(newLoginCommand(app), newLogoutCommand(app))
	addSyncCommands(root, app)
	addPublishCommands(root, app)
	return &rootCommand{Command: root}
}

func writeLines(output io.Writer, lines ...string) {
	for _, line := range lines {
		_, _ = io.WriteString(output, line+"\n")
	}
}
