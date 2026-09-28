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
	"github.com/ziyue67/tms/go-backend/internal/nodehub"
	"github.com/ziyue67/tms/go-backend/internal/store"
	"github.com/ziyue67/tms/go-backend/internal/verification"
)

type DataStore interface {
	Ping(context.Context) error
	UserByLogin(context.Context, string) (store.User, error)
	UpdatePassword(context.Context, int64, string) error
	ConfigValue(context.Context, string) (store.SiteConfig, error)
	Configs(context.Context) ([]store.SiteConfig, error)
	UpsertConfigs(context.Context, map[string]string) error
	UserByEmail(context.Context, string) (store.User, error)
	UsernameExists(context.Context, string) (bool, error)
	CountUsersByEmailDomain(context.Context, string) (int64, error)
	CreateUser(context.Context, store.NewUser) (store.User, error)
	NodeBySecret(context.Context, string) (store.NodeConnection, error)
	UpdateNodeConnection(context.Context, int64, string, int, int, int) error
	MarkNodeOffline(context.Context, int64) (bool, error)
	Nodes(context.Context) ([]store.Node, error)
	NodeByID(context.Context, int64) (store.Node, error)
	CreateNode(context.Context, store.NodeInput) (store.Node, error)
	UpdateNode(context.Context, int64, store.NodeInput) error
	RenameNode(context.Context, int64, string) (bool, error)
	DeleteNode(context.Context, int64) error
	UserByID(context.Context, int64) (store.User, error)
	Users(context.Context) ([]store.User, error)
	UsernameExistsExcept(context.Context, string, int64) (bool, error)
	CreateManagedUser(context.Context, store.ManagedUserInput) error
	UpdateManagedUser(context.Context, int64, store.ManagedUserInput) error
	UpdateUsernamePassword(context.Context, int64, string, string) error
	ResetUserFlow(context.Context, int64) (bool, error)
	ResetUserTunnelFlow(context.Context, int64) (bool, error)
	UserCleanupCommands(context.Context, int64) ([]store.CleanupCommand, error)
	DeleteUserCascade(context.Context, int64) error
	UserPackage(context.Context, int64) (store.UserPackage, error)
	SubscriptionPlans(context.Context, bool) ([]store.SubscriptionPlan, error)
	SubscriptionPlanByID(context.Context, int64) (store.SubscriptionPlan, error)
	CreateSubscriptionPlan(context.Context, store.PlanInput) (store.SubscriptionPlan, error)
	UpdateSubscriptionPlan(context.Context, int64, store.PlanInput) (store.SubscriptionPlan, error)
	DisableSubscriptionPlan(context.Context, int64) (store.SubscriptionPlan, error)
	DeleteSubscriptionPlan(context.Context, int64) (bool, error)
	RedeemCodes(context.Context, *int64, *int) ([]store.RedeemCode, error)
	GenerateRedeemCodes(context.Context, int64, string, int) ([]string, error)
	RevokeRedeemCode(context.Context, int64) error
	DeleteRedeemCode(context.Context, int64) error
	CurrentSubscription(context.Context, int64, bool) (*store.UserSubscription, error)
	SubscriptionAudit(context.Context, int64) ([]store.QuotaLog, error)
	AdjustSubscription(context.Context, int64, map[string]any) (*store.UserSubscription, error)
	RemoveSubscription(context.Context, int64) error
	ResetSubscriptionQuota(context.Context, int64) (*store.UserSubscription, error)
	RedeemSubscription(context.Context, int64, string) (*store.UserSubscription, error)
	SubscriptionDashboard(context.Context, int64) (map[string]any, error)
}

type VerificationService interface {
	CheckSendRate(string, string, verification.Purpose) error
	SendRegistration(context.Context, string, string) error
	SendResetAfterRateCheck(context.Context, string) error
	Consume(context.Context, string, string, verification.Purpose) bool
	SendTest(context.Context, string) error
	Audit(context.Context, int64) []string
	Health(context.Context) map[string]any
}

type Dependencies struct {
	Config       config.Config
	Store        DataStore
	Tokens       *auth.TokenService
	Redis        *redis.Client
	Verification VerificationService
	NodeHub      *nodehub.Hub
	Logger       *slog.Logger
	BuildCommit  string
	BuildTime    string
}

type API struct {
	config       config.Config
	store        DataStore
	tokens       *auth.TokenService
	redis        *redis.Client
	verification VerificationService
	nodeHub      *nodehub.Hub
	logger       *slog.Logger
	buildCommit  string
	buildTime    string
}

func New(dependencies Dependencies) http.Handler {
	api := &API{
		config:       dependencies.Config,
		store:        dependencies.Store,
		tokens:       dependencies.Tokens,
		redis:        dependencies.Redis,
		verification: dependencies.Verification,
		nodeHub:      dependencies.NodeHub,
		logger:       dependencies.Logger,
		buildCommit:  dependencies.BuildCommit,
		buildTime:    dependencies.BuildTime,
	}
	if api.logger == nil {
		api.logger = slog.Default()
	}

	router := chi.NewRouter()
	router.Use(cors, recoverer(api.logger), requestLogger(api.logger))

	router.HandleFunc("/flow/test", api.liveness)
	if api.nodeHub != nil {
		router.Handle("/system-info", api.nodeHub)
	}
	router.Get("/health/live", api.liveHealth)
	router.Get("/health/ready", api.readyHealth)

	router.Route("/api/v1", func(v1 chi.Router) {
		v1.Post("/user/login", api.login)
		v1.Post("/auth/login", api.login)
		v1.Get("/auth/config", api.authConfig)
		v1.Post("/auth/send-register-code", api.sendRegisterCode)
		v1.Post("/auth/send-reset-code", api.sendResetCode)
		v1.Post("/auth/forgot-password", api.sendResetCode)
		v1.Post("/auth/reset-password", api.resetPassword)
		v1.Post("/auth/register", api.register)
		v1.Post("/config/get", api.publicConfigValue)

		v1.Group(func(authenticated chi.Router) {
			authenticated.Use(authenticate(api.tokens))
			authenticated.Post("/config/list", api.publicConfigs)
			authenticated.Post("/user/package", api.userPackage)
			authenticated.Delete("/user/account", api.deleteCurrentUser)
			authenticated.Post("/user/updatePassword", api.updateCurrentPassword)
			authenticated.Get("/subscription/plans", api.publicSubscriptionPlans)
			authenticated.Get("/subscription/current", api.currentSubscription)
			authenticated.Get("/subscription/dashboard", api.subscriptionDashboard)
			authenticated.Post("/subscription/redeem", api.redeemSubscription)

			authenticated.Group(func(admin chi.Router) {
				admin.Use(requireAdmin)
				admin.Post("/config/private-list", api.privateConfigs)
				admin.Post("/config/update", api.updateConfigs)
				admin.Post("/config/update-single", api.updateSingleConfig)
				admin.Post("/admin/email/test", api.testEmail)
				admin.Get("/admin/email/audit", api.emailAudit)
				admin.Get("/admin/email/health", api.emailHealth)
				admin.Post("/node/create", api.createNode)
				admin.Post("/node/list", api.listNodes)
				admin.Post("/node/update", api.updateNode)
				admin.Post("/node/rename", api.renameNode)
				admin.Post("/node/delete", api.deleteNode)
				admin.Post("/node/install", api.nodeInstallCommand)
				admin.Post("/user/create", api.createUser)
				admin.Post("/user/list", api.listUsers)
				admin.Post("/user/update", api.updateUser)
				admin.Post("/user/delete", api.deleteUser)
				admin.Post("/user/reset", api.resetUserFlow)
				admin.Get("/admin/subscription/plans", api.adminSubscriptionPlans)
				admin.Post("/admin/subscription/plans", api.createSubscriptionPlan)
				admin.Put("/admin/subscription/plans/{id}", api.updateSubscriptionPlan)
				admin.Post("/admin/subscription/plans/{id}/disable", api.disableSubscriptionPlan)
				admin.Delete("/admin/subscription/plans/{id}", api.deleteSubscriptionPlan)
				admin.Post("/admin/subscription/redeem-codes", api.createRedeemCodes)
				admin.Get("/admin/subscription/redeem-codes", api.listRedeemCodes)
				admin.Post("/admin/subscription/redeem-codes/{id}/revoke", api.revokeRedeemCode)
				admin.Delete("/admin/subscription/redeem-codes/{id}", api.deleteRedeemCode)
				admin.Get("/admin/subscription/users/{userId}", api.adminUserSubscription)
				admin.Get("/admin/subscription/users/{userId}/audit", api.subscriptionAudit)
				admin.Put("/admin/subscription/users/{userId}", api.adjustSubscription)
				admin.Delete("/admin/subscription/users/{userId}", api.removeSubscription)
				admin.Post("/admin/subscription/users/{userId}/reset-quota", api.resetSubscriptionQuota)
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
