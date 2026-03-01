package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"syscall"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// runAdmin dispatches admin subcommands (reset-password, create-user, list-users).
func runAdmin(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printAdminHelp()
		return nil
	}

	switch args[0] {
	case "reset-password":
		return runAdminResetPassword(args[1:])
	case "create-user":
		return runAdminCreateUser(args[1:])
	case "list-users":
		return runAdminListUsers(args[1:])
	default:
		printAdminHelp()
		return fmt.Errorf("unknown admin command: %s", args[0])
	}
}

func printAdminHelp() {
	fmt.Fprintf(os.Stderr, `Usage: codeforge admin <command> [options]

Commands:
  reset-password   Reset a user's password
  create-user      Create a new user
  list-users       List all users
  help             Show this help message

Examples:
  codeforge admin reset-password --email admin@localhost
  codeforge admin reset-password --email admin@localhost --password NewPass123!
  codeforge admin create-user --email new@test.com --name "New Admin" --admin
  codeforge admin list-users
`)
}

func loadAdminDeps() (*service.AuthService, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}

	store := postgres.NewStore(pool)
	authSvc := service.NewAuthService(store, &cfg.Auth)

	cleanup := func() {
		pool.Close()
	}
	return authSvc, cleanup, nil
}

func runAdminResetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	email := fs.String("email", "", "user email address (required)")
	password := fs.String("password", "", "new password (prompted if not provided)") //nolint:gosec // CLI flag
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *email == "" {
		return fmt.Errorf("--email is required")
	}

	newPass := *password
	if newPass == "" {
		var err error
		newPass, err = promptPassword("New password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		confirm, err := promptPassword("Confirm password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		if newPass != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}

	authSvc, cleanup, err := loadAdminDeps()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	if err := authSvc.AdminResetPassword(ctx, *email, middleware.DefaultTenantID, newPass); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Password reset successfully for %s\n", *email)
	return nil
}

func runAdminCreateUser(args []string) error {
	fs := flag.NewFlagSet("create-user", flag.ContinueOnError)
	email := fs.String("email", "", "user email address (required)")
	name := fs.String("name", "", "user display name (required)")
	password := fs.String("password", "", "password (prompted if not provided)") //nolint:gosec // CLI flag
	admin := fs.Bool("admin", false, "grant admin role")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *email == "" {
		return fmt.Errorf("--email is required")
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	pass := *password
	if pass == "" {
		var err error
		pass, err = promptPassword("Password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		confirm, err := promptPassword("Confirm password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		if pass != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}

	role := user.RoleEditor
	if *admin {
		role = user.RoleAdmin
	}

	authSvc, cleanup, err := loadAdminDeps()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	u, err := authSvc.Register(ctx, &user.CreateRequest{
		Email:    *email,
		Name:     *name,
		Password: pass,
		Role:     role,
		TenantID: middleware.DefaultTenantID,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "User created: %s (id=%s, role=%s)\n", u.Email, u.ID, u.Role)
	return nil
}

func runAdminListUsers(args []string) error {
	fs := flag.NewFlagSet("list-users", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	authSvc, cleanup, err := loadAdminDeps()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	users, err := authSvc.ListUsers(ctx, middleware.DefaultTenantID)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	if len(users) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tENABLED\tMUST_CHANGE_PW")
	for i := range users {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%t\n",
			users[i].ID, users[i].Email, users[i].Name, users[i].Role, users[i].Enabled, users[i].MustChangePassword)
	}
	return w.Flush()
}

// promptPassword reads a password from the terminal without echoing.
func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(syscall.Stdin)) //nolint:unconvert // int conversion needed on some platforms
	fmt.Fprintln(os.Stderr)                         // newline after password input
	if err != nil {
		return "", err
	}
	return string(b), nil
}
