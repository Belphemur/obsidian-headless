package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/Belphemur/obsidian-headless/internal/buildinfo"
	configpkg "github.com/Belphemur/obsidian-headless/internal/config"
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
	viper.SetDefault("log-level", "info")
	viper.SetEnvPrefix("OBSIDIAN")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	root := &cobra.Command{
		Use:           "ob",
		Short:         "Headless Go client for Obsidian Sync and Publish",
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.logger = app.initLogger()
			app.configManager = configpkg.NewConfigManager(app.logger)
			return nil
		},
	}
	root.SetIn(app.stdin)
	root.SetOut(app.stdout)
	root.SetErr(app.stderr)
	root.PersistentFlags().String("api-base", viper.GetString("api-base"), "Obsidian API base URL")
	root.PersistentFlags().Int("timeout", viper.GetInt("timeout"), "HTTP timeout in seconds")
	root.PersistentFlags().String("log-level", viper.GetString("log-level"), "Log level (debug, info, warn, error, fatal, panic, disabled, trace)")
	_ = viper.BindPFlag("api-base", root.PersistentFlags().Lookup("api-base"))
	_ = viper.BindPFlag("timeout", root.PersistentFlags().Lookup("timeout"))
	_ = viper.BindPFlag("log-level", root.PersistentFlags().Lookup("log-level"))
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

func readPassword(input io.Reader) (string, error) {
	termFD, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(termFD.Fd())) {
		var password string
		_, _ = fmt.Scanln(&password)
		return password, nil
	}
	passwordBytes, err := term.ReadPassword(int(termFD.Fd()))
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}

type cliSpinner struct {
	w       io.Writer
	msg     string
	done    chan struct{}
	stopped chan struct{}
	tick    *time.Ticker
	once    sync.Once
}

func newSpinner(w io.Writer, msg string) *cliSpinner {
	return &cliSpinner{w: w, msg: msg}
}

func (s *cliSpinner) Start() {
	// Only animate if output is a terminal
	if f, ok := s.w.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
		return
	}
	s.done = make(chan struct{})
	s.stopped = make(chan struct{})
	s.tick = time.NewTicker(100 * time.Millisecond)
	go func() {
		defer close(s.stopped)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			select {
			case <-s.tick.C:
				_, _ = fmt.Fprintf(s.w, "\r%s %s", frames[i%len(frames)], s.msg)
				i++
			case <-s.done:
				s.tick.Stop()
				clearLen := utf8.RuneCountInString(frames[0]) + 1 + utf8.RuneCountInString(s.msg)
				_, _ = fmt.Fprintf(s.w, "\r%s\r", strings.Repeat(" ", clearLen))
				return
			}
		}
	}()
}

func (s *cliSpinner) Stop() {
	s.once.Do(func() {
		if s.done == nil {
			return
		}
		close(s.done)
		<-s.stopped
	})
}
