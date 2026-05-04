package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	apipkg "github.com/Belphemur/obsidian-headless/internal/api"
)

func newLoginCommand(app *App) *cobra.Command {
	var email string
	var password string
	var mfa string
	var acceptDisclaimer bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in to an Obsidian account",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if already logged in
			existingToken, err := app.configManager.LoadAuthToken()
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

			// Show disclaimer and require acceptance
			if !acceptDisclaimer {
				_, _ = fmt.Fprint(app.stdout, `╔══════════════════════════════════════════════════════════════╗
║                       !! DISCLAIMER !!                       ║
║                                                              ║
║  Obsidian Headless Go is a third-party, community-           ║
║  maintained tool. It is NOT an official Obsidian product,    ║
║  and it is NOT supported by Obsidian.                        ║
║                                                              ║
║  If you encounter any issues, do NOT contact Obsidian        ║
║  support. Instead, please open an issue on GitHub:           ║
║  https://github.com/Belphemur/obsidian-headless/issues       ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝
`)
				_, _ = fmt.Fprint(app.stdout, "Do you understand and accept this disclaimer? (y/N): ")
				var response string
				_, _ = fmt.Fscanln(app.stdin, &response)
				if !strings.HasPrefix(strings.ToLower(response), "y") {
					return fmt.Errorf("login cancelled: you must accept the disclaimer to proceed")
				}
			}

			// If already logged in, sign out old session
			if existingToken != "" {
				_ = app.client().SignOut(cmd.Context(), existingToken)
				_ = app.configManager.ClearAuthToken()
			}

			// Get credentials from flags or prompt
			if email == "" {
				_, _ = fmt.Fprint(app.stdout, "Email: ")
				_, _ = fmt.Scanln(&email)
			}
			if password == "" {
				_, _ = fmt.Fprint(app.stdout, "Password: ")
				pass, err := readPassword(app.stdin)
				if err != nil {
					return err
				}
				password = pass
				_, _ = fmt.Fprintln(app.stdout)
			}
			if email == "" || password == "" {
				return fmt.Errorf("both --email and --password are required")
			}

			// Attempt login
			sp := newSpinner(app.stderr, "Logging in...")
			sp.Start()
			response, err := app.client().SignIn(cmd.Context(), email, password, mfa)
			sp.Stop()
			if err != nil {
				// Check if 2FA is required using APIError type (server returns 200 with error in body)
				var apiErr *apipkg.APIError
				if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Message), "2fa") && mfa == "" {
					writeLines(app.stdout, "")
					_, _ = fmt.Fprint(app.stdout, "2FA code: ")
					_, _ = fmt.Scanln(&mfa)
					if mfa != "" {
						sp := newSpinner(app.stderr, "Logging in...")
						sp.Start()
						response, err = app.client().SignIn(cmd.Context(), email, password, mfa)
						sp.Stop()
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

			if err := app.configManager.SaveAuthToken(response.Token); err != nil {
				return err
			}
			writeLines(app.stdout, "Login successful.")
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "account email")
	command.Flags().StringVar(&password, "password", "", "account password")
	command.Flags().StringVar(&mfa, "mfa", "", "MFA code")
	command.Flags().BoolVar(&acceptDisclaimer, "accept-disclaimer", false, "accept the third-party disclaimer non-interactively")

	return command
}

func newLogoutCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the current account",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := app.configManager.LoadAuthToken()
			if err != nil {
				return err
			}
			if token != "" {
				_ = app.client().SignOut(cmd.Context(), token)
			}
			if err := app.configManager.ClearAuthToken(); err != nil {
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
