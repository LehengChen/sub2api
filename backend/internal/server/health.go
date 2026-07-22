package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	defaultReadinessTimeout = 2 * time.Second
	defaultShutdownTimeout  = 30 * time.Second
)

type healthProbe func(context.Context) error

// HealthService owns the process lifecycle state and the dependency probes
// used by /livez and /readyz. Probe errors are deliberately not returned in
// HTTP responses because driver errors can contain connection details.
type HealthService struct {
	readinessTimeout  time.Duration
	shutdownTimeout   time.Duration
	postgresProbe     healthProbe
	redisProbe        healthProbe
	migrationsProbe   healthProbe
	runtimeControl    runtimecontrol.Control
	workerFence       *service.WorkerFence
	schedulerProbe    healthProbe
	initialized       atomic.Bool
	draining          atomic.Bool
	activityMu        sync.Mutex
	activeRequests    int64
	activeConnections int64
	drained           chan struct{}
}

type ReadinessReport struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func NewHealthService(db *sql.DB, redisClient *redis.Client, cfg *config.Config, control runtimecontrol.Control, workerFence *service.WorkerFence, scheduler *service.SchedulerSnapshotService) *HealthService {
	readinessTimeout := defaultReadinessTimeout
	shutdownTimeout := defaultShutdownTimeout
	if cfg != nil {
		if cfg.Server.ReadinessTimeoutSeconds > 0 {
			readinessTimeout = time.Duration(cfg.Server.ReadinessTimeoutSeconds) * time.Second
		}
		if cfg.Server.ShutdownTimeoutSeconds > 0 {
			shutdownTimeout = time.Duration(cfg.Server.ShutdownTimeoutSeconds) * time.Second
		}
	}

	health := newHealthService(
		readinessTimeout,
		shutdownTimeout,
		func(ctx context.Context) error {
			if db == nil {
				return errors.New("postgres client is not initialized")
			}
			return db.PingContext(ctx)
		},
		func(ctx context.Context) error {
			if redisClient == nil {
				return errors.New("redis client is not initialized")
			}
			return redisClient.Ping(ctx).Err()
		},
		func(ctx context.Context) error {
			return repository.CheckMigrations(ctx, db)
		},
	)
	health.runtimeControl = control
	health.workerFence = workerFence
	if scheduler != nil {
		health.schedulerProbe = func(context.Context) error { return scheduler.InitialRebuildReady() }
	}
	return health
}

func newHealthService(
	readinessTimeout time.Duration,
	shutdownTimeout time.Duration,
	postgresProbe healthProbe,
	redisProbe healthProbe,
	migrationsProbe healthProbe,
) *HealthService {
	if readinessTimeout <= 0 {
		readinessTimeout = defaultReadinessTimeout
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	return &HealthService{
		readinessTimeout: readinessTimeout,
		shutdownTimeout:  shutdownTimeout,
		postgresProbe:    postgresProbe,
		redisProbe:       redisProbe,
		migrationsProbe:  migrationsProbe,
		drained:          closedSignal(),
	}
}

func closedSignal() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (s *HealthService) MarkInitialized() {
	if s != nil {
		s.initialized.Store(true)
	}
}

func (s *HealthService) BeginDrain() {
	if s != nil {
		s.draining.Store(true)
	}
}

func (s *HealthService) IsDraining() bool {
	return s != nil && s.draining.Load()
}

func (s *HealthService) ShutdownTimeout() time.Duration {
	if s == nil || s.shutdownTimeout <= 0 {
		return defaultShutdownTimeout
	}
	return s.shutdownTimeout
}

// TrackRequests records ordinary HTTP handlers. It intentionally excludes
// health endpoints so a load balancer probe cannot keep drain open forever.
// Hijacked WebSocket connections leave the HTTP handler before the socket is
// closed; those handlers must use RegisterLongLivedConnection instead.
func (s *HealthService) TrackRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isHealthPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		s.beginActivity(false)
		defer s.endActivity(false)
		c.Next()
	}
}

// RegisterLongLivedConnection returns a release function for a hijacked or
// otherwise externally managed connection (for example a WebSocket). The
// release function is idempotent and should be deferred by the owner.
func (s *HealthService) RegisterLongLivedConnection() func() {
	if s == nil {
		return func() {}
	}
	s.beginActivity(true)
	var once sync.Once
	return func() {
		once.Do(func() { s.endActivity(true) })
	}
}

func (s *HealthService) beginActivity(connection bool) {
	if s == nil {
		return
	}
	s.activityMu.Lock()
	if s.activeRequests+s.activeConnections == 0 {
		s.drained = make(chan struct{})
	}
	if connection {
		s.activeConnections++
	} else {
		s.activeRequests++
	}
	s.activityMu.Unlock()
}

func (s *HealthService) endActivity(connection bool) {
	if s == nil {
		return
	}
	s.activityMu.Lock()
	wasActive := s.activeRequests+s.activeConnections > 0
	if connection {
		if s.activeConnections > 0 {
			s.activeConnections--
		}
	} else if s.activeRequests > 0 {
		s.activeRequests--
	}
	if wasActive && s.activeRequests+s.activeConnections == 0 {
		close(s.drained)
	}
	s.activityMu.Unlock()
}

func (s *HealthService) ActiveRequests() int {
	if s == nil {
		return 0
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	return int(s.activeRequests)
}

func (s *HealthService) ActiveConnections() int {
	if s == nil {
		return 0
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	return int(s.activeConnections)
}

// WaitForDrain waits for both tracked HTTP requests and explicitly registered
// long-lived connections. It never waits beyond ctx's deadline.
func (s *HealthService) WaitForDrain(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.activityMu.Lock()
	if s.activeRequests+s.activeConnections == 0 {
		s.activityMu.Unlock()
		return nil
	}
	drained := s.drained
	s.activityMu.Unlock()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *HealthService) Readiness(ctx context.Context) (ReadinessReport, bool) {
	checks := map[string]string{
		"initialization": "ok",
		"drain":          "ok",
		"role":           "ok",
		"worker_fence":   "ok",
		"scheduler":      "not_required",
		"postgres":       "not_checked",
		"redis":          "not_checked",
		"migrations":     "not_checked",
	}
	if s == nil || !s.initialized.Load() {
		checks["initialization"] = "failed"
		return ReadinessReport{Status: "not_ready", Checks: checks}, false
	}
	if s.draining.Load() {
		checks["drain"] = "failed"
		return ReadinessReport{Status: "not_ready", Checks: checks}, false
	}

	ready := true
	if s.runtimeControl.Role != "" && !s.runtimeControl.TrafficEligible() {
		checks["role"] = "failed"
		ready = false
	}
	if s.runtimeControl.RequiresWorkerLease() && (s.workerFence == nil || !s.workerFence.Valid()) {
		checks["worker_fence"] = "failed"
		ready = false
	}

	probeCtx, cancel := context.WithTimeout(ctx, s.readinessTimeout)
	defer cancel()

	probes := []struct {
		name  string
		probe healthProbe
	}{
		{name: "postgres", probe: s.postgresProbe},
		{name: "redis", probe: s.redisProbe},
		{name: "migrations", probe: s.migrationsProbe},
	}
	if s.schedulerProbe != nil && (s.runtimeControl.Role == "" || s.runtimeControl.RunsWorkers()) {
		checks["scheduler"] = "not_checked"
		probes = append(probes, struct {
			name  string
			probe healthProbe
		}{name: "scheduler", probe: s.schedulerProbe})
	}
	for _, item := range probes {
		if item.probe == nil || item.probe(probeCtx) != nil {
			checks[item.name] = "failed"
			ready = false
			continue
		}
		checks[item.name] = "ok"
	}

	status := "ready"
	if !ready {
		status = "not_ready"
	}
	return ReadinessReport{Status: status, Checks: checks}, ready
}

func (s *HealthService) LivenessHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *HealthService) ReadinessHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	report, ready := s.Readiness(c.Request.Context())
	if !ready {
		c.JSON(http.StatusServiceUnavailable, report)
		return
	}
	c.JSON(http.StatusOK, report)
}

// DrainGuard rejects requests that have not entered the handler chain before
// draining began. Existing requests continue and are bounded by Server.Shutdown.
func (s *HealthService) DrainGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.IsDraining() || isHealthPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Retry-After", "1")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
	}
}

func isHealthPath(path string) bool {
	switch path {
	case "/health", "/livez", "/readyz":
		return true
	default:
		return false
	}
}
