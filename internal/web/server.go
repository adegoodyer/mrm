// Package web is mrm's local web UI: an overview of every repo with live
// updates, a page per repo, the run history and settings. It binds to
// 127.0.0.1 only and every request needs this run's token; it drives the
// same engine, lock, cache and history as the CLI and the TUI.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/discover"
	"github.com/adegoodyer/mrm/internal/engine"
	"github.com/adegoodyer/mrm/internal/forgeapi"
	"github.com/adegoodyer/mrm/internal/model"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/runner"
	"github.com/adegoodyer/mrm/internal/store"
)

//go:embed static templates
var assets embed.FS

const (
	cookieName  = "mrm_token"
	tokenHeader = "X-MRM-Token"
)

// Options configure a web UI session.
type Options struct {
	Config     *config.Config
	Engine     *engine.Engine
	Workspaces []*config.Workspace // shown; all when empty
	Port       int                 // for the Host check; set from the listener when 0
	Token      string              // generated when empty
	Repos      []model.Repo        // skip discovery (tests)
	NoRefresh  bool                // don't read status on start (tests)
	CachePath  string
	HistPath   string
	LockDir    string
	Forge      *forgeapi.Client // PR and CI info; nil leaves it out
}

// Server is one web UI session.
type Server struct {
	opts  Options
	cfg   *config.Config
	run   *runner.Runner
	tmpl  *template.Template
	hub   *hub
	token string
	ctx   context.Context

	mu        sync.Mutex
	repos     []model.Repo
	results   map[string]model.Result  // by path
	forge     map[string]forgeapi.Info // by path
	forgeBusy atomic.Bool
	byID      map[string]string // id -> path
	running   map[string]bool
	busy      string // label of our running operation
	progress  [2]int
	external  string // another mrm holds a sync lock
	lastSync  time.Time
	cacheSeen time.Time // cache mtime last loaded or written by us
}

// New loads the repos (from the cache, so the first page is immediate).
func New(ctx context.Context, opts Options) (*Server, error) {
	if opts.CachePath == "" {
		opts.CachePath = paths.StatusCache()
	}
	if opts.HistPath == "" {
		opts.HistPath = paths.History()
	}
	if opts.LockDir == "" {
		opts.LockDir = paths.LockDir()
	}
	if opts.Token == "" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		opts.Token = hex.EncodeToString(b)
	}
	tmpl, err := template.New("").ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{
		opts: opts, cfg: opts.Config, tmpl: tmpl, hub: newHub(), token: opts.Token, ctx: ctx,
		run:     &runner.Runner{Eng: opts.Engine, CachePath: opts.CachePath, HistPath: opts.HistPath, LockDir: opts.LockDir},
		results: map[string]model.Result{}, forge: map[string]forgeapi.Info{}, byID: map[string]string{}, running: map[string]bool{},
	}
	s.repos = opts.Repos
	if s.repos == nil {
		wss := opts.Workspaces
		if len(wss) == 0 {
			wss, _ = s.cfg.Select(nil)
		}
		s.repos, _ = discover.All(s.cfg, wss)
	}
	s.loadCache()
	for _, r := range s.repos {
		s.byID[repoID(r.Path)] = r.Path
		if opts.Forge != nil {
			if i, ok := opts.Forge.CachedFor(forgeTarget(s.results[r.Path])); ok {
				s.forge[r.Path] = i
			}
		}
	}
	if run, ok := store.LastRun(opts.HistPath, "sync"); ok {
		s.lastSync = run.Started.Add(time.Duration(run.DurationMS) * time.Millisecond)
	}
	return s, nil
}

// Token is this session's access token.
func (s *Server) Token() string { return s.token }

// Serve serves on ln until ctx ends: it reads every repo's status in the
// background and watches for runs by other mrm processes.
func (s *Server) Serve(ln net.Listener) error {
	if s.opts.Port == 0 {
		if addr, ok := ln.Addr().(*net.TCPAddr); ok {
			s.opts.Port = addr.Port
		}
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-s.ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if !s.opts.NoRefresh {
		go s.start("status", s.repos, false)
	}
	go s.watch()
	err := srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Handler is the whole web UI, behind the host check, security headers and
// token check.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServerFS(static))))
	mux.HandleFunc("GET /{$}", s.overview)
	mux.HandleFunc("GET /rows", s.rows)
	mux.HandleFunc("GET /repo/{id}", s.repoPage)
	mux.HandleFunc("GET /activity", s.activity)
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("POST /api/{action}", s.action)
	return s.checkHost(securityHeaders(s.auth(mux)))
}

// checkHost refuses any Host but this machine's loopback names: a page on
// another site that resolves its own name to 127.0.0.1 (DNS rebinding)
// can't reach the API.
func (s *Server) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil {
			host, port = r.Host, ""
		}
		okHost := host == "127.0.0.1" || host == "localhost" || host == "::1"
		okPort := port == "" || port == strconv.Itoa(s.opts.Port)
		if !okHost || !okPort {
			http.Error(w, "mrm web only answers on 127.0.0.1", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// auth: the first visit carries ?token=, which becomes an HttpOnly cookie
// (and is dropped from the address bar). Every request then needs the
// cookie; every POST also needs the token in a header, which a page on
// another site can't send.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if t := r.URL.Query().Get("token"); t != "" {
			if !s.validToken(t) {
				s.deny(w)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			q := r.URL.Query()
			q.Del("token")
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.validToken(c.Value) {
			s.deny(w)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.validToken(r.Header.Get(tokenHeader)) {
			http.Error(w, "missing "+tokenHeader+" header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validToken(t string) bool {
	return t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func (s *Server) deny(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = s.tmpl.ExecuteTemplate(w, "denied", nil)
}

func cacheFor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache") // revalidate: assets change between versions
		next.ServeHTTP(w, r)
	})
}
