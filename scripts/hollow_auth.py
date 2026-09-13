"""Shared login helper for the Hollowmere smoke scripts.

The world only upgrades authenticated WebSockets, so every script needs a
real account. These helpers register (or log in) over HTTP and hand back
the Cookie header to present on the upgrade.

The scripts use throwaway accounts with a fixed password. That is fine for
a local stack and a deliberate reason not to point them at the live host.
"""
import json
import secrets
import time
import urllib.error
import urllib.request
from urllib.parse import urlsplit, urlunsplit

SESSION_COOKIE = "hollowmere_session"
DEFAULT_PASSWORD = "smoke-hollow-tester-7"


def http_base(ws_url):
    """Map ws://host/ws to http://host (and wss:// to https://)."""
    parts = urlsplit(ws_url)
    scheme = "https" if parts.scheme in ("wss", "https") else "http"
    return urlunsplit((scheme, parts.netloc, "", "", ""))


def _post(base, path, payload):
    req = urllib.request.Request(
        base + path,
        data=json.dumps(payload).encode(),
        # No Origin header on purpose. These are not browsers, and a
        # deployment with HOLLOWMERE_ALLOWED_ORIGINS set would reject
        # http://127.0.0.1 as a foreign origin. The server allows an
        # absent Origin precisely for non-browser callers like this one.
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, resp.headers.get_all("Set-Cookie") or [], b""
    except urllib.error.HTTPError as err:
        return err.code, [], err.read()


def _token(cookies):
    for raw in cookies:
        for part in raw.split(";"):
            name, _, value = part.strip().partition("=")
            if name == SESSION_COOKIE and value:
                return value
    return None


def _post_with_retry(base, path, payload, attempts=6):
    """POST, backing off when the login throttle refuses us.

    Running a smoke script a few times in a row is exactly what the
    per-account guess budget is meant to notice, so wait it out rather
    than asking anyone to loosen the limit.
    """
    delay = 1.0
    for attempt in range(attempts):
        status, cookies, body = _post(base, path, payload)
        if status != 429:
            return status, cookies, body
        if attempt < attempts - 1:
            time.sleep(delay)
            delay = min(delay * 2, 12.0)
    return status, cookies, body


def login(ws_url, username, password=DEFAULT_PASSWORD):
    """Ensure an account exists and return its Cookie header value."""
    base = http_base(ws_url)

    status, cookies, body = _post_with_retry(
        base, "/auth/register", {"username": username, "password": password}
    )
    token = _token(cookies)
    if token:
        return f"{SESSION_COOKIE}={token}"

    if status != 409:
        raise RuntimeError(
            f"register {username}: HTTP {status} {body.decode(errors='replace').strip()}"
        )

    # The account is left over from an earlier run; log in instead.
    status, cookies, body = _post_with_retry(
        base, "/auth/login", {"username": username, "password": password}
    )
    token = _token(cookies)
    if not token:
        raise RuntimeError(
            f"login {username}: HTTP {status} {body.decode(errors='replace').strip()}"
        )
    return f"{SESSION_COOKIE}={token}"


# Closed v0 palette — same ids the world accepts. A smoke script that
# skips this still gets in; the hamlet just sees the default paperdoll.
_DEFAULT_LOOKS = {
    "skin": "tan",
    "hair": "short",
    "hairColor": "umber",
    "top": "moss",
}


def carve_looks(ws_url, cookie, looks=None):
    """First-write the appearance creator. Spam-safe: a second call is a no-op."""
    base = http_base(ws_url)
    req = urllib.request.Request(
        base + "/auth/looks",
        data=json.dumps(looks or _DEFAULT_LOOKS).encode(),
        headers={"Content-Type": "application/json", "Cookie": cookie},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status
    except urllib.error.HTTPError as err:
        if err.code in (200, 201):
            return err.code
        raise RuntimeError(
            f"looks: HTTP {err.code} {err.read().decode(errors='replace').strip()}"
        )


def fresh_name(prefix):
    """A per-run account name, for scripts that assert on an empty pack.

    Accounts persist now, so a fixed name would carry the previous run's
    berries into this one. Names cap at 16 characters.
    """
    return (prefix[:9] + secrets.token_hex(3))[:16]


async def connect(ws_url, cookie):
    """websockets.connect carrying the session cookie.

    The keyword changed name in websockets 14, so try the new one first.
    """
    import websockets

    # Cookie only: see the note in _post about omitting Origin.
    headers = {"Cookie": cookie}
    try:
        return await websockets.connect(ws_url, additional_headers=headers)
    except TypeError:
        return await websockets.connect(ws_url, extra_headers=headers)


async def join(ws_url, username, password=DEFAULT_PASSWORD):
    """Log in, open an authenticated socket, and send the hello frame.

    Returns (ws, cookie). The hello frame carries no identity: the cookie
    on the upgrade already settled who this socket is.
    """
    cookie = login(ws_url, username, password)
    carve_looks(ws_url, cookie)
    ws = await connect(ws_url, cookie)
    await ws.send(json.dumps({"t": "hello"}))
    return ws, cookie
