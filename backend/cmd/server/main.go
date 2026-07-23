package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/runtimecontrol"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds (set by ldflags)
)

func init() {
	// 如果 Version 已通过 ldflags 注入（例如 -X main.Version=...），则不要覆盖。
	if strings.TrimSpace(Version) != "" {
		return
	}

	// 默认从 embedded VERSION 文件读取版本号（编译期打包进二进制）。
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

// initLogger configures the default slog handler based on gin.Mode().
// In non-release mode, Debug level logs are enabled.
func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	// Parse command line flags
	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	migrateMode := flag.Bool("migrate", false, "Apply database migrations and exit")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	runtimeControl, err := runtimecontrol.LoadFromEnv()
	if err != nil {
		log.Fatalf("Invalid process runtime control: %v", err)
	}
	deploymentControl, err := service.LoadDeploymentControlFromEnv()
	if err != nil {
		log.Fatalf("Invalid deployment control: %v", err)
	}
	// Enforce the external ownership boundary before setup/AUTO_SETUP can
	// perform any database or bootstrap writes. Explicit roles are the only
	// allowed production entry points; migration-only remains a separate role.
	if err := validateStartupPolicy(runtimeControl, deploymentControl, *setupMode, *migrateMode); err != nil {
		log.Fatal(err)
	}

	// CLI setup mode is available only to self-managed installations.
	if *setupMode {
		if err := setup.RunCLI(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	if *migrateMode || runtimeControl.Role == runtimecontrol.RoleMigrator {
		runMigrator()
		return
	}

	// Check if setup is needed
	if setup.NeedsSetup() {
		if runtimeControl.Role != runtimecontrol.RoleAll {
			log.Fatalf("Setup must be completed before starting explicit process role %q", runtimeControl.Role)
		}
		// Check if auto-setup is enabled (for Docker deployment)
		if setup.AutoSetupEnabled() {
			log.Println("Auto setup mode enabled...")
			if err := setup.AutoSetupFromEnv(); err != nil {
				log.Fatalf("Auto setup failed: %v", err)
			}
			// Continue to main server after auto-setup
		} else {
			log.Println("First run detected, starting setup wizard...")
			runSetupServer()
			return
		}
	}

	// Normal server mode
	runMainServer(runtimeControl, deploymentControl)
}

func validateStartupPolicy(runtimeControl runtimecontrol.Control, deploymentControl service.DeploymentControl, setupMode bool, migrateMode bool) error {
	if !deploymentControl.IsExternallyManaged() {
		return nil
	}
	if runtimeControl.Role == runtimecontrol.RoleAll {
		return fmt.Errorf(
			"externally managed deployments must set %s to an explicit non-legacy role; upgrade legacy runners before activating this image",
			runtimecontrol.ProcessRoleEnv,
		)
	}
	if setupMode {
		return errors.New("externally managed deployments cannot run setup mode")
	}
	if migrateMode && runtimeControl.Role != runtimecontrol.RoleMigrator {
		return errors.New("externally managed migrations must use the migrator process role")
	}
	return nil
}

func runMigrator() {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("Failed to load config for migrator: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := repository.RunMigrations(ctx, cfg); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}
	log.Println("Database migrations completed")
}

func runSetupServer() {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(config.CORSConfig{}))
	r.Use(middleware.SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil))

	// Register setup routes
	setup.RegisterRoutes(r)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	// Get server address from config.yaml or environment variables (SERVER_HOST, SERVER_PORT)
	// This allows users to run setup on a different address if needed
	addr := config.GetServerAddress()
	log.Printf("Setup wizard available at http://%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		Protocols:         protocols,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start setup server: %v", err)
	}
}

func runMainServer(runtimeControl runtimecontrol.Control, deploymentControl service.DeploymentControl) {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("⚠️  WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	buildInfo := handler.BuildInfo{
		Version:           Version,
		Commit:            Commit,
		BuildType:         BuildType,
		DeploymentControl: deploymentControl,
		RuntimeControl:    runtimeControl,
	}

	app, err := initializeApplication(buildInfo)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer func() {
		if err := app.WorkerFence.Stop(); err != nil {
			log.Printf("Worker fence release failed: %v", err)
		}
	}()
	defer app.Cleanup()
	if runtimeControl.TrafficEligible() && app.PromptAudit != nil {
		if err := app.PromptAudit.Start(context.Background()); err != nil {
			// Startup continues so unrelated APIs stay up. Fail-closed (unavailable)
			// applies only when a persisted blocking policy was observed; without
			// blocking intent, Prompt Audit stays ModeOff so the gateway remains
			// usable and administrators can still disable the feature (#4560).
			log.Printf("Prompt Audit started in degraded state: %v", err)
		}
	}
	app.Health.MarkInitialized()
	if !runtimeControl.ServesHTTP() {
		log.Printf("Process role %s started without an HTTP listener", runtimeControl.Role)
		waitForTermination(app)
		return
	}

	// 启动服务器
	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started on %s", app.Server.Addr)

	waitForTermination(app)
	drainAndShutdown(app)

	log.Println("Server exited")
}

func waitForTermination(app *Application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	if app.WorkerFence != nil && app.WorkerFence.Required() {
		select {
		case <-quit:
		case <-app.WorkerFence.Lost():
			log.Println("Singleton worker fence lost; draining process")
		}
		return
	}
	<-quit
}

func drainAndShutdown(app *Application) {
	// Become unready before closing listeners so the load balancer stops
	// assigning new work while existing requests are allowed to drain.
	app.Health.BeginDrain()
	log.Println("Server is draining...")

	ctx, cancel := context.WithTimeout(context.Background(), app.Health.ShutdownTimeout())
	defer cancel()

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Printf("Graceful shutdown deadline reached: %v", err)
		if closeErr := app.Server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			log.Printf("Forced server close failed: %v", closeErr)
		}
	}
	if err := app.Health.WaitForDrain(ctx); err != nil {
		log.Printf("Long-lived connection drain deadline reached: %v (requests=%d connections=%d)", err, app.Health.ActiveRequests(), app.Health.ActiveConnections())
		if closeErr := app.Server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			log.Printf("Forced server close after drain failed: %v", closeErr)
		}
	}
}
