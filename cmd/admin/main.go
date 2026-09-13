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
//	docker compose exec world /app/admin grant Kyle admin
//	docker compose exec world /app/admin ban Kyle 24h
//	docker compose exec world /app/admin mute Kyle 15m
//	docker compose exec world /app/admin audit
//
// Every command writes an audit row. Role grants are host-only by design;
// see docs/adr/0001-admin-role.md.
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
		runReset(ctx, svc, pg, firstArg(args))
	case "revoke":
		runRevoke(ctx, svc, pg, firstArg(args))
	case "grant":
		runGrant(ctx, svc, pg, firstArg(args), secondArg(args))
	case "ban":
		runBan(ctx, svc, pg, firstArg(args), secondArg(args))
	case "unban":
		runBan(ctx, svc, pg, firstArg(args), "0")
	case "mute":
		runMute(ctx, svc, pg, rd, firstArg(args), secondArg(args))
	case "audit":
		fs := flag.NewFlagSet("audit", flag.ExitOnError)
		n := fs.Int("n", 30, "entries to show")
		_ = fs.Parse(args)
		runAudit(ctx, pg, *n)
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

func secondArg(args []string) string {
	if len(args) < 2 {
		return ""
	}
	return args[1]
}

// audit records what the operator just did. A failure here is reported but
// does not undo the action — a silent gap in the log is worse than a noisy
// one, and pretending the action did not happen would be a lie.
func audit(ctx context.Context, pg *store.Postgres, action, targetID, targetName, detail string) {
	err := pg.RecordAdminAction(ctx, store.AdminAction{
		ActorName:  "host",
		Action:     action,
		Target:     &targetID,
		TargetName: targetName,
		Detail:     detail,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: action succeeded but was not written to the audit log: %v\n", err)
	}
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
	fmt.Fprintln(w, "NAME\tROLE\tSTATE\tCREATED\tLAST LOGIN")
	for _, a := range accounts {
		last := "never"
		if a.LastLogin != nil {
			last = a.LastLogin.Format("2006-01-02 15:04")
		}
		state := "ok"
		if a.BannedUntil != nil && a.BannedUntil.After(time.Now()) {
			state = "banned until " + a.BannedUntil.Format("01-02 15:04")
		}
		if a.PlayerID == "" {
			state = "no character"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			a.Username, a.Role, state, a.CreatedAt.Format("2006-01-02"), last)
	}
	_ = w.Flush()
	if total > len(accounts) {
		fmt.Printf("\n%d of %d accounts (-n 0 for all)\n", len(accounts), total)
	} else {
		fmt.Printf("\n%d account(s)\n", total)
	}
}

func runReset(ctx context.Context, svc *auth.Service, pg *store.Postgres, name string) {
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

	audit(ctx, pg, "reset", accountID, username, "")

	fmt.Printf("\n  %s can now log in with:\n\n      %s\n\n", username, pw)
	fmt.Println("  Shown once. Every session for this account has been signed out,")
	fmt.Println("  and any open socket drops within about ten seconds.")
	fmt.Println("  Have them change it from the Account panel after logging in.")
}

func runRevoke(ctx context.Context, svc *auth.Service, pg *store.Postgres, name string) {
	if name == "" {
		fail("revoke needs an account name")
	}
	accountID, username := lookup(ctx, svc, name)
	if err := svc.RevokeSessions(ctx, accountID); err != nil {
		fail("revoke %s: %v", username, err)
	}
	audit(ctx, pg, "revoke", accountID, username, "")
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

func runGrant(ctx context.Context, svc *auth.Service, pg *store.Postgres, name, role string) {
	if name == "" || role == "" {
		fail("grant needs a name and a role (player, moderator, admin)")
	}
	if !auth.ValidRole(role) {
		fail("unknown role %q — use player, moderator or admin", role)
	}
	accountID, username := lookup(ctx, svc, name)
	if err := svc.SetRole(ctx, accountID, role); err != nil {
		fail("grant %s: %v", username, err)
	}
	audit(ctx, pg, "grant", accountID, username, role)
	fmt.Printf("%s is now a %s\n", username, role)
}

func runBan(ctx context.Context, svc *auth.Service, pg *store.Postgres, name, dur string) {
	if name == "" {
		fail("ban needs an account name")
	}
	accountID, username := lookup(ctx, svc, name)

	if dur == "" {
		dur = "24h"
	}
	d, err := time.ParseDuration(dur)
	if err != nil {
		fail("%q is not a duration (try 30m, 24h, 168h)", dur)
	}
	if d <= 0 {
		if err := svc.Ban(ctx, accountID, nil); err != nil {
			fail("unban %s: %v", username, err)
		}
		audit(ctx, pg, "unban", accountID, username, "")
		fmt.Printf("%s may return to the hamlet\n", username)
		return
	}
	until := time.Now().Add(d)
	if err := svc.Ban(ctx, accountID, &until); err != nil {
		fail("ban %s: %v", username, err)
	}
	audit(ctx, pg, "ban", accountID, username, d.String())
	fmt.Printf("%s is barred until %s; signed out everywhere, and any open\n"+
		"socket drops within about ten seconds\n", username, until.Format("2006-01-02 15:04 MST"))
}

func runMute(ctx context.Context, svc *auth.Service, pg *store.Postgres, rd *store.Redis, name, dur string) {
	if name == "" {
		fail("mute needs an account name")
	}
	accountID, username := lookup(ctx, svc, name)
	if dur == "" {
		dur = "15m"
	}
	d, err := time.ParseDuration(dur)
	if err != nil {
		fail("%q is not a duration (try 5m, 1h)", dur)
	}
	if err := rd.SetMute(ctx, accountID, d); err != nil {
		fail("mute %s: %v", username, err)
	}
	if d <= 0 {
		audit(ctx, pg, "unmute", accountID, username, "")
		fmt.Printf("%s can speak again\n", username)
		return
	}
	audit(ctx, pg, "mute", accountID, username, d.String())
	fmt.Printf("%s is quiet for %s (takes effect within about ten seconds)\n", username, d)
}

func runAudit(ctx context.Context, pg *store.Postgres, limit int) {
	entries, err := pg.RecentAdminActions(ctx, limit)
	if err != nil {
		fail("read audit log: %v", err)
	}
	if len(entries) == 0 {
		fmt.Println("nothing in the audit log yet")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tWHO\tDID\tTO\tDETAIL")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.CreatedAt.Format("2006-01-02 15:04"), e.ActorName, e.Action, e.TargetName, e.Detail)
	}
	_ = w.Flush()
}

func usage() {
	fmt.Fprint(os.Stderr, `hollowmere admin

  admin list [-n 40]       accounts, newest first
  admin reset <name>       new random password, signs the account out everywhere
  admin revoke <name>      sign out everywhere, leaving the password alone
  admin grant <name> <role>  player | moderator | admin
  admin ban <name> [24h]   bar the account and evict it
  admin unban <name>       lift a ban
  admin mute <name> [15m]  quiet an account in chat
  admin audit [-n 30]      who did what, newest first

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
