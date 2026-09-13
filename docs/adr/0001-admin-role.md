# ADR-0001: Admin role delegates from host access

**Status:** Accepted
**Date:** 2026-09-13
**Deciders:** thegliffy

## Context

Hollowmere has real accounts as of the auth work, and one recovery path:
`cmd/admin`, a CLI that talks straight to Postgres and Redis. It performs no
login of its own. Its authorization is host access — if you can reach the
Docker socket on moneta you can run it, and you could equally have opened
`psql` and rewritten the rows by hand.

That is proportionate today, with one operator who also owns the server. Three
things break it:

1. **Delegation.** Somebody should be able to quiet a disruptive player without
   being handed SSH and the database.
2. **Accountability.** Nothing records that a password was reset, or by whom.
   With one operator that is tolerable; with two it is not.
3. **Timeliness.** Moderation over SSH is fine for a forgotten password and
   useless for something happening in the hamlet right now.

The forces pulling the other way: this is a proof of concept with a handful of
accounts, and every privileged surface added is one that can be phished,
leaked, or left switched on. The world is a single authoritative goroutine, so
anything touching live play has to route through the hub rather than poke the
database behind its back.

## Decision

Add a role to accounts, an append-only audit log, and the two moderation verbs
that reuse machinery already present (ban, mute). Keep the destructive verbs —
password reset, role grants — on the host CLI. Defer the HTTP endpoints and the
browser panel until a second operator actually exists.

**The role system delegates authority from host access; it does not replace
it.** The first admin must be granted from the host, because nobody inside the
application has standing to create one. `cmd/admin` therefore remains both the
bootstrap and the break-glass path.

## Options Considered

### Option A: Keep host access as the only authority

| Dimension | Assessment |
|-----------|------------|
| Complexity | Low — nothing to build |
| Cost | Zero |
| Scalability | Fails at the second operator |
| Team familiarity | Already in use |

**Pros:** No new privileged surface. No new credential to steal. The blast
radius is exactly the one that already exists.
**Cons:** No delegation, no audit trail, and no way to act on something
happening in-world without opening a terminal.

### Option B: Full in-app admin — roles, HTTP endpoints, browser panel

| Dimension | Assessment |
|-----------|------------|
| Complexity | High — new endpoints, new UI, new session semantics |
| Cost | Largest surface area of the three |
| Scalability | Fits a real player base |
| Team familiarity | Reuses existing auth patterns |

**Pros:** Moderation without shell access; the whole story in one place.
**Cons:** Builds a delegation system before anyone needs delegating to. Every
endpoint is a new thing to gate, rate-limit and get right. An admin session
becomes as valuable as the admin password, so it needs re-auth or short-lived
elevation to be honest — more machinery, for one account.

### Option C (chosen): Role and audit now, moderation verbs that reuse existing paths, endpoints deferred

| Dimension | Assessment |
|-----------|------------|
| Complexity | Medium — one migration, one new table, two enforcement points |
| Cost | Small; most of it is schema |
| Scalability | Carries to a real player base without rework |
| Team familiarity | Reuses session revocation and the heartbeat loop |

**Pros:** Schema changes land while there is one account to migrate, which is
the cheapest they will ever be. The audit log starts recording the CLI's own
actions immediately, which is where the accountability gap actually is today.
Ban and mute are close to free given what already exists.
**Cons:** Moderation still requires SSH until the endpoints land. Roles exist
before anything reads them over HTTP, which looks like dead weight until it
isn't.

## Trade-off Analysis

The decisive question is not *what could an admin do* but *when does delegation
become real*. Today it is not: one person owns the server, the database and the
only account. Option B builds the delegation machinery anyway and pays for it
in permanently larger attack surface.

Against that, schema changes get more expensive with every account. Adding
`role`, `banned_until` and `admin_actions` against a single row is trivial;
against a live player base it is a migration with a rollback plan. So the
cheapest ordering is schema early, behaviour late — which is Option C.

Two verbs are worth pulling forward because they cost almost nothing:

- **Ban** falls out of session revocation. `Auth.Resolve` already fails closed,
  and the heartbeat loop already evicts sockets whose session has gone, so a
  ban evicts within about ten seconds with no new eviction path.
- **Mute** is a single check before `TryChat`, with state in Redis under a TTL
  so it expires on its own and survives a restart.

Password reset and role grants stay host-only on purpose. Keeping the
destructive verbs behind SSH means an admin session compromise costs a kick and
a mute, not the account table. It also avoids the re-auth machinery Option B
would need to be defensible.

## Consequences

**Easier**
- Delegating moderation later: the column, the audit table and the enforcement
  points will already be in place, so it becomes an endpoint rather than a
  redesign.
- Answering "who reset that password" — from the first CLI invocation, not from
  whenever the panel ships.
- Banning: reuses revocation, no new eviction path.

**Harder**
- Moderation still needs a terminal until the endpoints land. This is a
  deliberate deferral, not an oversight.
- One more schema surface to keep the Go migrator and `migrations/001_init.sql`
  agreed on.

**To revisit**
- **Handle resolution de-anonymises.** Peers see opaque handles that rotate per
  join, specifically so they cannot correlate each other across sessions. Any
  admin view mapping handle back to account punches through that, and a
  compromised admin account de-anonymises everyone at once. That cost lands
  when the endpoints land — not before, because nothing resolves handles yet.
- **Admin session value.** If `/admin/*` ever gains destructive verbs, the
  session needs re-auth or short-lived elevation first.
- **Role escalation.** Grants are host-only so an admin compromise cannot mint
  more admins. Revisit only with a strong reason.

## Action Items

1. [ ] `schema_migrations` version table — three schema changes at once is
       exactly the case it exists for, and its absence is a known gap.
2. [ ] `accounts.role` (`player` | `moderator` | `admin`) and
       `accounts.banned_until`.
3. [ ] `admin_actions` append-only table, with actor and target names
       denormalised so rows stay readable after an account is deleted.
4. [ ] CLI: `admin grant <name> <role>`, `admin ban`/`unban`, `admin mute`;
       every existing command writes an audit row with `actor_name='host'`.
5. [ ] Enforce: ban fails `Auth.Resolve`; mute checked before `TryChat`.
6. [ ] Defer — `/admin/*` endpoints and the browser panel, until a second
       operator exists who should not have SSH.
