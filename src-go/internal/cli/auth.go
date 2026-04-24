package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	apipkg "github.com/Belphemur/obsidian-headless/src-go/internal/api"
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
			// Check if already logged in
			existingToken, err := configpkg.LoadAuthToken()
			if err != nil {
				return err
			}

			// If logged in and no credentials provided, show current user info
			if existingToken != "" && email == "" && password == "" {
				userInfo, err := app.client().UserInfo(cmd.Context(), existingToken)
				if err == nil {
					writeLines(app.stdout, fmt.Sprintf("Logged in as: %s", userInfo.Email))
					if userInfo.Name != "" {
						writeLines(app.stdout, fmt.Sprintf("Name: %s", userInfo.Name))
					}
					return nil
				}
				// Token may be invalid, proceed with login
			}

			// If already logged in, sign out old session
			if existingToken != "" {
				_ = app.client().SignOut(cmd.Context(), existingToken)
				_ = configpkg.ClearAuthToken()
			}

			// Get credentials from flags or prompt
			if email == "" {
				fmt.Fprint(app.stdout, "Email: ")
				fmt.Scanln(&email)
			}
			if password == "" {
				fmt.Fprint(app.stdout, "Password: ")
				pass, err := readPassword(app.stdin)
				if err != nil {
					return err
				}
				password = pass
			}
			if email == "" || password == "" {
				return fmt.Errorf("both --email and --password are required")
			}

			// Attempt login
			response, err := app.client().SignIn(cmd.Context(), email, password, mfa)
			if err != nil {
				// Check if 2FA is required using APIError type (server returns 200 with error in body)
				var apiErr *apipkg.APIError
				if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Message), "2fa") && mfa == "" {
					fmt.Fprint(app.stdout, "2FA code: ")
					fmt.Scanln(&mfa)
					if mfa != "" {
						response, err = app.client().SignIn(cmd.Context(), email, password, mfa)
						if err != nil {
							return fmt.Errorf("login failed: %w", err)
						}
					} else {
						return fmt.Errorf("login failed: %w", err)
					}
				} else {
					return fmt.Errorf("login failed: %w", err)
				}
			}

			if err := configpkg.SaveAuthToken(response.Token); err != nil {
				return err
			}
			writeLines(app.stdout, "Login successful.")
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "account email")
	command.Flags().StringVar(&password, "password", "", "account password")
	command.Flags().StringVar(&mfa, "mfa", "", "MFA code")

	viper.BindPFlag("email", command.Flags().Lookup("email"))
	viper.BindPFlag("password", command.Flags().Lookup("password"))
	viper.BindEnv("email", "OBSIDIAN_EMAIL")
	viper.BindEnv("password", "OBSIDIAN_PASSWORD")

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

var _ = func() {
	cobra.OnInitialize(func() {
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	})
}
