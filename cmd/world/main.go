package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/thegliffy/2007mmo/internal/hub"
	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/store"
	"github.com/thegliffy/2007mmo/internal/world"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("hollowmere ")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := env("DATABASE_URL", "postgres://hollowmere:hollowmere@127.0.0.1:5432/hollowmere?sslmode=disable")
	redisURL := env("REDIS_URL", "redis://127.0.0.1:6379")
	httpAddr := env("HTTP_ADDR", ":8080")
	tlsAddr := os.Getenv("TLS_ADDR")
	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")
	webDir := env("WEB_DIR", findWeb())
	tickMs := envInt("TICK_MS", protocol.TickMs)

	pg, err := store.NewPostgres(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pg.Close()

	rd, err := store.NewRedis(redisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rd.Close()

	w := world.New(pg)
	seed := make([]world.NodeRec, 0, len(w.Nodes))
	for _, n := range w.Nodes {
		seed = append(seed, world.NodeRec{ID: n.ID, Kind: n.Kind, X: n.X, Y: n.Y, Remaining: n.Remaining, Cooldown: n.Cooldown})
	}
	if err := pg.SeedNodes(ctx, seed); err != nil {
		log.Fatalf("seed nodes: %v", err)
	}
	existing, err := pg.LoadNodes(ctx)
	if err != nil {
		log.Fatalf("load nodes: %v", err)
	}
	w.RestoreNodes(existing)

	h := hub.New(w, pg, rd, time.Duration(tickMs)*time.Millisecond)
	go h.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("/health", h.ServeHealth)
	mux.HandleFunc("/stats", h.ServeStats)
	mux.HandleFunc("/metrics", h.ServeMetrics)
	mux.Handle("/", noStoreClient(http.FileServer(http.Dir(webDir))))

	srv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("http %s  web=%s  tick=%dms  world=%s", httpAddr, webDir, tickMs, protocol.WorldName)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	if tlsAddr != "" && tlsCert != "" && tlsKey != "" {
		tlsSrv := &http.Server{
			Addr:              tlsAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		}
		go func() {
			log.Printf("wss  %s", tlsAddr)
			if err := tlsSrv.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
				log.Fatalf("tls: %v", err)
			}
		}()
		defer func() { _ = tlsSrv.Shutdown(context.Background()) }()
	}

	<-ctx.Done()
	log.Printf("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

// Live static hosting (and some browsers) keep a week-old app.js after a
// compose rebuild. HTML/JS/CSS must revalidate so combat click fixes land.
func noStoreClient(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Ext(r.URL.Path) {
		case "", ".html", ".js", ".css":
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}

func findWeb() string {
	cands := []string{"./client", "../client", "/app/web", "web"}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(c, "index.html")); err == nil {
				return c
			}
		}
	}
	return "./client"
}
