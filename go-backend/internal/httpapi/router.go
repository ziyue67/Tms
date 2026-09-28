package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type DataStore interface {
	Ping(context.Context) error
	UserByLogin(context.Context, string) (store.User, error)
	UpdatePassword(context.Context, int64, string) error
	ConfigValue(context.Context, string) (store.SiteConfig, error)
	Configs(context.Context) ([]store.SiteConfig, error)
	UpsertConfigs(context.Context, map[string]string) error
}

type Dependencies struct {
	Config      config.Config
	Store       DataStore
	Tokens      *auth.TokenService
	Redis       *redis.Client
	Logger      *slog.Logger
	BuildCommit string
	BuildTime   string
}

type API struct {
	config      config.Config
	store       DataStore
	tokens      *auth.TokenService
	redis       *redis.Client
	logger      *slog.Logger
	buildCommit string
	buildTime   string
}

func New(dependencies Dependencies) http.Handler {
	api := &API{
		config:      dependencies.Config,
		store:       dependencies.Store,
		tokens:      dependencies.Tokens,
		redis:       dependencies.Redis,
		logger:      dependencies.Logger,
		buildCommit: dependencies.BuildCommit,
		buildTime:   dependencies.BuildTime,
	}
	if api.logger == nil {
		api.logger = slog.Default()
	}

	router := chi.NewRouter()
	router.Use(cors, recoverer(api.logger), requestLogger(api.logger))

	router.HandleFunc("/flow/test", api.liveness)
	router.Get("/health/live", api.liveHealth)
	router.Get("/health/ready", api.readyHealth)

	router.Route("/api/v1", func(v1 chi.Router) {
		v1.Post("/user/login", api.login)
		v1.Post("/auth/login", api.login)
		v1.Get("/auth/config", api.authConfig)
		v1.Post("/config/get", api.publicConfigValue)

		v1.Group(func(authenticated chi.Router) {
			authenticated.Use(authenticate(api.tokens))
			authenticated.Post("/config/list", api.publicConfigs)

			authenticated.Group(func(admin chi.Router) {
				admin.Use(requireAdmin)
				admin.Post("/config/private-list", api.privateConfigs)
				admin.Post("/config/update", api.updateConfigs)
				admin.Post("/config/update-single", api.updateSingleConfig)
			})
		})
	})

	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeResponseStatus(w, http.StatusNotFound, Error(404, "接口不存在"))
	})
	return router
}

func (a *API) liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("test"))
}

func (a *API) liveHealth(w http.ResponseWriter, _ *http.Request) {
	writeResponse(w, OK(map[string]any{
		"status":      "up",
		"buildCommit": a.buildCommit,
		"buildTime":   a.buildTime,
	}))
}

func (a *API) readyHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]bool{"database": a.store.Ping(ctx) == nil}
	checks["redis"] = a.redis != nil && a.redis.Ping(ctx).Err() == nil
	ready := checks["database"] && checks["redis"]
	if !ready {
		writeResponseStatus(w, http.StatusServiceUnavailable, OK(map[string]any{"ready": ready, "checks": checks}))
		return
	}
	writeResponse(w, OK(map[string]any{"ready": ready, "checks": checks}))
}
