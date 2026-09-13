package hub

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/protocol"
)

// maxAuthBody caps a credential post. Passwords are capped at 128 bytes,
// so anything this size is someone probing.
const maxAuthBody = 4 << 10

// Routes registers every auth endpoint plus the world's own handlers.
func (h *Hub) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("/health", h.ServeHealth)
	mux.HandleFunc("/stats", h.ServeStats)
	mux.HandleFunc("/metrics", h.ServeMetrics)
	mux.HandleFunc("/auth/register", h.ServeRegister)
	mux.HandleFunc("/auth/login", h.ServeLogin)
	mux.HandleFunc("/auth/logout", h.ServeLogout)
	mux.HandleFunc("/auth/me", h.ServeMe)
	mux.HandleFunc("/auth/password", h.ServePassword)
}

// sessionToken reads the login cookie.
func sessionToken(r *http.Request) string {
	c, err := r.Cookie(protocol.SessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// setSessionCookie writes the session as HttpOnly so no script — ours or
// injected — can read it, and SameSite=Lax so it is not attached to
// cross-site requests.
func (h *Hub) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     protocol.SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.proxy.cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionTTL / time.Second),
	})
}

func (h *Hub) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     protocol.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.proxy.cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// sameOrigin guards the cookie-authenticated POST endpoints against
// cross-site submission. SameSite=Lax already blocks the common case;
// this closes the rest without a CSRF token round-trip.
//
// Requiring application/json is part of the defense: an HTML form can
// only send urlencoded, multipart, or text/plain bodies, so a form on a
// hostile page cannot reach these handlers at all.
func (h *Hub) sameOrigin(r *http.Request) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "application/json") {
		return false
	}
	return h.sameOriginNoBody(r)
}

// sameOriginNoBody is the origin half of the check, for posts that carry
// no body. It reuses the same allowlist the WebSocket handshake uses, and
// falls back to Referer when a caller sends no Origin.
func (h *Hub) sameOriginNoBody(r *http.Request) bool {
	if r.Header.Get("Origin") == "" {
		ref := r.Header.Get("Referer")
		if ref == "" {
			return true
		}
		u, err := url.Parse(ref)
		if err != nil || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}
		probe := r.Clone(r.Context())
		probe.Header.Set("Origin", u.Scheme+"://"+u.Host)
		return h.proxy.allowOrigin(probe)
	}
	return h.proxy.allowOrigin(r)
}

func writeAuthJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func authFail(w http.ResponseWriter, code int, msg string) {
	writeAuthJSON(w, code, protocol.AuthError{Error: msg})
}

// decodeAuth reads and validates the envelope shared by every auth POST.
func (h *Hub) decodeAuth(w http.ResponseWriter, r *http.Request) (*protocol.AuthRequest, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		authFail(w, http.StatusMethodNotAllowed, "post only")
		return nil, false
	}
	if !h.sameOrigin(r) {
		authFail(w, http.StatusForbidden, "that request did not come from Hollowmere")
		return nil, false
	}
	var req protocol.AuthRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		authFail(w, http.StatusBadRequest, "that was not a well-formed request")
		return nil, false
	}
	return &req, true
}

// authThrottle applies the per-IP and per-name password-guess budgets.
func (h *Hub) authThrottle(w http.ResponseWriter, r *http.Request, username string) bool {
	ip := h.proxy.clientIP(r)
	if !h.limits.login.allow(ip) {
		h.metrics.AddLimited("login")
		authFail(w, http.StatusTooManyRequests, "too many attempts. Wait a breath.")
		return false
	}
	if key := strings.ToLower(strings.TrimSpace(username)); key != "" {
		if !h.limits.loginUser.allow(key) {
			h.metrics.AddLimited("login")
			authFail(w, http.StatusTooManyRequests, "too many attempts on that name. Wait a breath.")
			return false
		}
	}
	return true
}

// isCredentialFailure reports whether an error means "you got the
// password wrong", as opposed to a name collision or a weak new password.
// Only the former belongs in the login-failure metric: counting signup
// collisions there would make a busy registration day look like an attack.
func isCredentialFailure(err error) bool {
	return errors.Is(err, auth.ErrBadCredentials)
}

// clientAuthError maps a service error onto a status and a message that
// is safe to show. Anything unrecognized becomes a 500 with no detail.
func clientAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrBadCredentials):
		authFail(w, http.StatusUnauthorized, auth.ErrBadCredentials.Error())
	case errors.Is(err, auth.ErrUsernameTaken):
		authFail(w, http.StatusConflict, auth.ErrUsernameTaken.Error())
	case errors.Is(err, auth.ErrBanned):
		// Say so plainly. A generic failure would read as a forgotten
		// password and send them round the reset loop for nothing.
		authFail(w, http.StatusForbidden, auth.ErrBanned.Error())
	case errors.Is(err, auth.ErrBadUsername), errors.Is(err, auth.ErrWeakPassword):
		authFail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, auth.ErrNoSession):
		authFail(w, http.StatusUnauthorized, "log in first")
	default:
		log.Printf("auth: %v", err)
		authFail(w, http.StatusInternalServerError, "the hamlet hiccuped. Try again.")
	}
}

func (h *Hub) ServeRegister(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeAuth(w, r)
	if !ok {
		return
	}
	if !h.authThrottle(w, r, req.Username) {
		return
	}
	_, _, username, token, err := h.Auth.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		if isCredentialFailure(err) {
			h.metrics.AddLoginFail()
		}
		clientAuthError(w, err)
		return
	}
	h.setSessionCookie(w, r, token)
	writeAuthJSON(w, http.StatusCreated, protocol.AuthResponse{Username: username, World: protocol.WorldName})
}

func (h *Hub) ServeLogin(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeAuth(w, r)
	if !ok {
		return
	}
	if !h.authThrottle(w, r, req.Username) {
		return
	}
	_, _, username, token, err := h.Auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if isCredentialFailure(err) {
			h.metrics.AddLoginFail()
		}
		clientAuthError(w, err)
		return
	}
	h.setSessionCookie(w, r, token)
	writeAuthJSON(w, http.StatusOK, protocol.AuthResponse{Username: username, World: protocol.WorldName})
}

func (h *Hub) ServeLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		authFail(w, http.StatusMethodNotAllowed, "post only")
		return
	}
	// No JSON body required to log out, but it still must be same-site.
	if !h.sameOriginNoBody(r) {
		authFail(w, http.StatusForbidden, "that request did not come from Hollowmere")
		return
	}
	token := sessionToken(r)
	if token != "" {
		if err := h.Auth.Logout(r.Context(), token); err != nil {
			log.Printf("auth: logout: %v", err)
		}
		h.dropSocketsForSession(token)
	}
	h.clearSessionCookie(w, r)
	writeAuthJSON(w, http.StatusOK, protocol.AuthResponse{World: protocol.WorldName})
}

func (h *Hub) ServeMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		authFail(w, http.StatusMethodNotAllowed, "get only")
		return
	}
	_, _, username, err := h.Auth.Resolve(r.Context(), sessionToken(r))
	if err != nil {
		h.clearSessionCookie(w, r)
		authFail(w, http.StatusUnauthorized, "log in first")
		return
	}
	writeAuthJSON(w, http.StatusOK, protocol.AuthResponse{Username: username, World: protocol.WorldName})
}

func (h *Hub) ServePassword(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeAuth(w, r)
	if !ok {
		return
	}
	token := sessionToken(r)
	accountID, _, username, err := h.Auth.Resolve(r.Context(), token)
	if err != nil {
		authFail(w, http.StatusUnauthorized, "log in first")
		return
	}
	if !h.authThrottle(w, r, username) {
		return
	}
	newToken, err := h.Auth.ChangePassword(r.Context(), accountID, req.Current, req.Next)
	if err != nil {
		if isCredentialFailure(err) {
			h.metrics.AddLoginFail()
		}
		clientAuthError(w, err)
		return
	}
	// Every other session for this account is now dead. Close their
	// sockets too, or a stolen cookie keeps playing until it disconnects.
	h.dropSocketsForAccountExcept(accountID, newToken)
	h.setSessionCookie(w, r, newToken)
	writeAuthJSON(w, http.StatusOK, protocol.AuthResponse{Username: username, World: protocol.WorldName})
}

// dropSocketsForSession closes any live WebSocket holding this token.
func (h *Hub) dropSocketsForSession(token string) {
	if token == "" {
		return
	}
	h.mu.Lock()
	var doomed []*Client
	for _, cl := range h.clients {
		if cl.session == token {
			doomed = append(doomed, cl)
		}
	}
	h.mu.Unlock()
	for _, cl := range doomed {
		h.sendJSON(cl, protocol.Err{T: protocol.MsgErr, Msg: "You have left the hamlet."})
		cl.stop()
	}
}

// dropSocketsForAccountExcept closes every socket for an account other
// than the one holding keep.
func (h *Hub) dropSocketsForAccountExcept(accountID, keep string) {
	if accountID == "" {
		return
	}
	h.mu.Lock()
	var doomed []*Client
	for _, cl := range h.clients {
		if cl.accountID == accountID && cl.session != keep {
			doomed = append(doomed, cl)
		}
	}
	h.mu.Unlock()
	for _, cl := range doomed {
		h.sendJSON(cl, protocol.Err{T: protocol.MsgErr, Msg: "Your password changed. Log in again."})
		cl.stop()
	}
}
