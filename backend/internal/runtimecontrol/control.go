package runtimecontrol

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Role string

const (
	RoleAll      Role = "all"
	RoleActive   Role = "active"
	RoleAPI      Role = "api"
	RoleWorker   Role = "worker"
	RoleStandby  Role = "standby"
	RoleMigrator Role = "migrator"
)

const (
	ProcessRoleEnv        = "SUB2API_PROCESS_ROLE"
	InstanceIDEnv         = "SUB2API_INSTANCE_ID"
	WorkerLeaseKeyEnv     = "SUB2API_WORKER_LEASE_KEY"
	WorkerLeaseTTLEnv     = "SUB2API_WORKER_LEASE_TTL_SECONDS"
	WorkerLeaseRenewEnv   = "SUB2API_WORKER_LEASE_RENEW_SECONDS"
	MultiAPIEnabledEnv    = "SUB2API_MULTI_API_ENABLED"
	DefaultWorkerLeaseKey = "sub2api:runtime:primary-worker"
)

var (
	instanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	leaseKeyPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._-]{0,199}$`)
)

type Control struct {
	Role                Role
	InstanceID          string
	WorkerLeaseKey      string
	WorkerLeaseTTL      time.Duration
	WorkerRenewInterval time.Duration
	MultiAPIEnabled     bool
}

func Default() Control {
	return Control{
		Role:                RoleAll,
		WorkerLeaseKey:      DefaultWorkerLeaseKey,
		WorkerLeaseTTL:      30 * time.Second,
		WorkerRenewInterval: 10 * time.Second,
	}
}

func LoadFromEnv() (Control, error) {
	control := Default()
	if raw := strings.TrimSpace(os.Getenv(ProcessRoleEnv)); raw != "" {
		control.Role = Role(strings.ToLower(raw))
	}
	control.InstanceID = strings.TrimSpace(os.Getenv(InstanceIDEnv))
	if control.InstanceID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return Control{}, fmt.Errorf("resolve process instance identity: %w", err)
		}
		control.InstanceID = strings.TrimSpace(hostname)
	}
	if raw := strings.TrimSpace(os.Getenv(WorkerLeaseKeyEnv)); raw != "" {
		control.WorkerLeaseKey = raw
	}

	var err error
	control.WorkerLeaseTTL, err = durationFromSecondsEnv(WorkerLeaseTTLEnv, control.WorkerLeaseTTL)
	if err != nil {
		return Control{}, err
	}
	control.WorkerRenewInterval, err = durationFromSecondsEnv(WorkerLeaseRenewEnv, control.WorkerRenewInterval)
	if err != nil {
		return Control{}, err
	}
	if raw := strings.TrimSpace(os.Getenv(MultiAPIEnabledEnv)); raw != "" {
		enabled, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return Control{}, fmt.Errorf("%s must be true or false", MultiAPIEnabledEnv)
		}
		control.MultiAPIEnabled = enabled
	}
	if err := control.Validate(); err != nil {
		return Control{}, err
	}
	return control, nil
}

func durationFromSecondsEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return time.Duration(seconds) * time.Second, nil
}

func (c Control) Validate() error {
	switch c.Role {
	case RoleAll, RoleActive, RoleAPI, RoleWorker, RoleStandby, RoleMigrator:
	default:
		return fmt.Errorf("%s must be one of all, active, api, worker, standby, or migrator", ProcessRoleEnv)
	}
	if !instanceIDPattern.MatchString(c.InstanceID) {
		return fmt.Errorf("%s must be a non-secret instance label using letters, digits, dot, underscore, or hyphen", InstanceIDEnv)
	}
	if !leaseKeyPattern.MatchString(c.WorkerLeaseKey) {
		return fmt.Errorf("%s contains invalid characters", WorkerLeaseKeyEnv)
	}
	if c.WorkerLeaseTTL <= 0 {
		return fmt.Errorf("%s must be positive", WorkerLeaseTTLEnv)
	}
	if c.WorkerRenewInterval <= 0 || c.WorkerRenewInterval*2 >= c.WorkerLeaseTTL {
		return fmt.Errorf("%s must be positive and less than half of %s", WorkerLeaseRenewEnv, WorkerLeaseTTLEnv)
	}
	return nil
}

func (c Control) ServesHTTP() bool {
	switch c.Role {
	case RoleAll, RoleActive, RoleAPI, RoleStandby:
		return true
	default:
		return false
	}
}

func (c Control) TrafficEligible() bool {
	switch c.Role {
	case RoleAll, RoleActive:
		return true
	case RoleAPI:
		return c.MultiAPIEnabled
	default:
		return false
	}
}

func (c Control) RunsWorkers() bool {
	switch c.Role {
	case RoleAll, RoleActive, RoleWorker:
		return true
	default:
		return false
	}
}

func (c Control) RequiresWorkerLease() bool {
	return c.Role == RoleActive || c.Role == RoleWorker
}

func (c Control) AppliesMigrations() bool {
	return c.Role == RoleAll || c.Role == RoleMigrator
}

func (c Control) AllowsBootstrapWrites() bool {
	return c.Role == RoleAll
}
