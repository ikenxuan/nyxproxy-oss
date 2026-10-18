package app

import "time"

type Status struct {
	Version       string           `json:"version"`
	UptimeSeconds int64            `json:"uptime_seconds"`
	Proxy         ProxyStatus      `json:"proxy"`
	Monitoring    MonitoringStatus `json:"monitoring"`
	Pool          PoolStatus       `json:"pool"`
	Metrics       MetricsStatus    `json:"metrics"`
	Config        ConfigStatus     `json:"config"`
	Platform      PlatformStatus   `json:"platform"`
	Apply         ApplyStatus      `json:"apply"`
}

type ProxyStatus struct {
	State     string     `json:"state"`
	Type      string     `json:"type"`
	Listen    string     `json:"listen"`
	Protocol  int        `json:"protocol"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Error     string     `json:"error"`
}

type MonitoringStatus struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
	Error   string `json:"error"`
}

type PoolStatus struct {
	Enabled              bool       `json:"enabled"`
	Size                 int        `json:"size"`
	Prefix               string     `json:"prefix"`
	Rotations            int64      `json:"rotations"`
	MaxUsage             int        `json:"max_usage"`
	MaxAgeMinutes        int        `json:"max_age_minutes"`
	UsedAddresses        int        `json:"used_addresses"`
	FreshAddresses       int        `json:"fresh_addresses"`
	TotalUsage           int64      `json:"total_usage"`
	AverageUsage         float64    `json:"average_usage"`
	Utilization          float64    `json:"utilization"`
	LastRotatedAt        *time.Time `json:"last_rotated_at,omitempty"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
}

// PoolAddress is the per-address view used by the address pool page.
type PoolAddress struct {
	Address             string    `json:"address"`
	Usage               int       `json:"usage"`
	LastUsed            time.Time `json:"last_used"`
	AddedAt             time.Time `json:"added_at"`
	RemainingUses       int       `json:"remaining_uses"`
	RemainingAgeSeconds int64     `json:"remaining_age_seconds"`
	State               string    `json:"state"`
}

// PoolDetails is returned by GET /api/pool.
type PoolDetails struct {
	Pool        PoolStatus    `json:"pool"`
	Addresses   []PoolAddress `json:"addresses"`
	GeneratedAt time.Time     `json:"generated_at"`
}

type MetricsStatus struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	TotalRequests     int64 `json:"total_requests"`
	BytesSent         int64 `json:"bytes_sent"`
	BytesReceived     int64 `json:"bytes_received"`
}

type LastApply struct {
	OK         bool      `json:"ok"`
	Error      string    `json:"error"`
	At         time.Time `json:"at"`
	Generation uint64    `json:"generation"`
}

type ConfigStatus struct {
	Path          string     `json:"path"`
	Generation    uint64     `json:"generation"`
	LastAppliedAt *time.Time `json:"last_applied_at,omitempty"`
	LastApply     LastApply  `json:"last_apply"`
}

type PlatformStatus struct {
	OS        string   `json:"os"`
	IPCommand bool     `json:"ip_command"`
	Warnings  []string `json:"warnings"`
}

type ApplyStatus struct {
	InProgress bool   `json:"in_progress"`
	Step       string `json:"step"`
}

type ApplyResult struct {
	Changed        bool      `json:"changed"`
	Generation     uint64    `json:"generation"`
	Restarted      Restarted `json:"restarted"`
	ReauthRequired bool      `json:"reauth_required"`
}

type Restarted struct {
	Proxy      bool `json:"proxy"`
	Monitoring bool `json:"monitoring"`
}
