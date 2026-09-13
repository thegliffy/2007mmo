// Command admin is the operator's side door: list accounts, reset a
// forgotten password, sign someone out everywhere.
//
// Hollowmere collects no email, so there is no self-service reset. This is
// the whole recovery story, and it deliberately requires access to the
// host — anyone who can run it can already read the database.
//
//	docker compose exec world /app/admin list
//	docker compose exec world /app/admin reset Kyle
//	docker compose exec world /app/admin revoke Kyle
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/store"
)

func main() {
	// Flags are parsed per subcommand so "admin list -n 3" works. The
	// top-level flag package stops at the first non-flag argument, which
	// silently ignored anything written after the command name.
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbURL := env("DATABASE_URL", "postgres://hollowmere:hollowmere@127.0.0.1:5432/hollowmere?sslmode=disable")
	redisURL := env("REDIS_URL", "redis://127.0.0.1:6379")

	pg, err := store.NewPostgres(ctx, dbURL)
	if err != nil {
		fail("postgres: %v", err)
	}
	defer pg.Close()

	rd, err := store.NewRedis(redisURL)
	if err != nil {
		fail("redis: %v", err)
	}
	defer rd.Close()

	svc := auth.NewService(pg, rd)

	switch cmd {
	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		n := fs.Int("n", 40, "max accounts to show (0 for all)")
		_ = fs.Parse(args)
		runList(ctx, pg, *n)
	case "reset":
		runReset(ctx, svc, firstArg(args))
	case "revoke":
		runRevoke(ctx, svc, firstArg(args))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func runList(ctx context.Context, pg *store.Postgres, limit int) {
	accounts, err := pg.ListAccounts(ctx)
	if err != nil {
		fail("list accounts: %v", err)
	}
	if len(accounts) == 0 {
		fmt.Println("no accounts yet")
		return
	}
	total := len(accounts)
	if limit > 0 && total > limit {
		accounts = accounts[:limit]
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCREATED\tLAST LOGIN\tCHARACTER")
	for _, a := range accounts {
		last := "never"
		if a.LastLogin != nil {
			last = a.LastLogin.Format("2006-01-02 15:04")
		}
		character := a.PlayerID
		if character == "" {
			character = "(none)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			a.Username, a.CreatedAt.Format("2006-01-02"), last, character)
	}
	_ = w.Flush()
	if total > len(accounts) {
		fmt.Printf("\n%d of %d accounts (-n 0 for all)\n", len(accounts), total)
	} else {
		fmt.Printf("\n%d account(s)\n", total)
	}
}

func runReset(ctx context.Context, svc *auth.Service, name string) {
	if name == "" {
		fail("reset needs an account name")
	}
	accountID, username := lookup(ctx, svc, name)

	pw, err := auth.GeneratePassword(username)
	if err != nil {
		fail("generate password: %v", err)
	}
	if err := svc.ResetPassword(ctx, accountID, pw); err != nil {
		// ResetPassword reports a partial success in its error text when
		// the password changed but sessions survived; pass that through
		// rather than implying nothing happened.
		fail("reset %s: %v", username, err)
	}

	fmt.Printf("\n  %s can now log in with:\n\n      %s\n\n", username, pw)
	fmt.Println("  Shown once. Every session for this account has been signed out,")
	fmt.Println("  and any open socket drops within about ten seconds.")
	fmt.Println("  Have them change it from the Account panel after logging in.")
}

func runRevoke(ctx context.Context, svc *auth.Service, name string) {
	if name == "" {
		fail("revoke needs an account name")
	}
	accountID, username := lookup(ctx, svc, name)
	if err := svc.RevokeSessions(ctx, accountID); err != nil {
		fail("revoke %s: %v", username, err)
	}
	fmt.Printf("signed %s out everywhere; the password is unchanged\n", username)
}

func lookup(ctx context.Context, svc *auth.Service, name string) (accountID, username string) {
	accountID, username, err := svc.FindAccount(ctx, name)
	if err != nil {
		if errors.Is(err, auth.ErrBadUsername) {
			fail("%q is not a valid account name", name)
		}
		fail("look up %s: %v", name, err)
	}
	if accountID == "" {
		fail("no account called %q (try: admin list)", name)
	}
	return accountID, username
}

func usage() {
	fmt.Fprint(os.Stderr, `hollowmere admin

  admin list [-n 40]     accounts, newest first
  admin reset <name>     set a new random password and sign the account out everywhere
  admin revoke <name>    sign the account out everywhere, leaving the password alone

Reads DATABASE_URL and REDIS_URL from the environment, which the world
container already sets:

  docker compose exec world /app/admin list
`)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "admin: "+format+"\n", args...)
	os.Exit(1)
}
