package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/viper"

	"github.com/Belphemur/obsidian-headless/src-go/internal/api"
	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
)

type App struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	root   command
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
	token, err := configpkg.LoadAuthToken()
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("no account logged in; run login first")
	}
	return token, nil
}
