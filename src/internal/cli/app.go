package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/Belphemur/obsidian-headless/src-go/internal/api"
	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/logging"
)

type App struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	root          command
	logger        zerolog.Logger
	configManager *configpkg.ConfigManager
}

type command interface {
	ExecuteContext(context.Context) error
	SetArgs([]string)
}

func New(stdin io.Reader, stdout, stderr io.Writer) *App {
	application := &App{stdin: stdin, stdout: stdout, stderr: stderr}
	application.root = newRootCommand(application)
	return application
}

func (a *App) initLogger() zerolog.Logger {
	levelStr := viper.GetString("log-level")
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	return logging.NewConsoleLogger(a.stderr)
}

func (a *App) Execute(ctx context.Context) error {
	return a.root.ExecuteContext(ctx)
}

func (a *App) ExecuteArgs(ctx context.Context, args []string) error {
	a.root.SetArgs(args)
	return a.root.ExecuteContext(ctx)
}

func (a *App) client() *api.Client {
	return api.New(viper.GetString("api-base"), time.Duration(viper.GetInt("timeout"))*time.Second)
}

func (a *App) requireToken() (string, error) {
	token, err := a.configManager.LoadAuthToken()
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("no account logged in; run login first")
	}
	return token, nil
}
