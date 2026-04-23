package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
)

func newLoginCommand(app *App) *cobra.Command {
	var email string
	var password string
	var mfa string
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in to an Obsidian account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" && password == "" {
				token, err := configpkg.LoadAuthToken()
				if err != nil {
					return err
				}
				if token == "" {
					return fmt.Errorf("provide --email and --password")
				}
				user, err := app.client().UserInfo(cmd.Context(), token)
				if err != nil {
					return err
				}
				writeLines(app.stdout, fmt.Sprintf("Logged in as %s <%s>", user.Name, user.Email))
				return nil
			}
			if email == "" || password == "" {
				return fmt.Errorf("both --email and --password are required")
			}
			response, err := app.client().SignIn(cmd.Context(), email, password, mfa)
			if err != nil {
				return err
			}
			if err := configpkg.SaveAuthToken(response.Token); err != nil {
				return err
			}
			writeLines(app.stdout, fmt.Sprintf("Logged in as %s <%s>", response.Name, response.Email))
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "account email")
	command.Flags().StringVar(&password, "password", "", "account password")
	command.Flags().StringVar(&mfa, "mfa", "", "MFA code")
	return command
}

func newLogoutCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the current account",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := configpkg.LoadAuthToken()
			if err != nil {
				return err
			}
			if token != "" {
				_ = app.client().SignOut(cmd.Context(), token)
			}
			if err := configpkg.ClearAuthToken(); err != nil {
				return err
			}
			writeLines(app.stdout, "Logged out.")
			return nil
		},
	}
}
