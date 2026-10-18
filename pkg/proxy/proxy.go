package proxy

import (
	"context"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
)

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopped  State = "stopped"
	StateError    State = "error"
)

type ProxyStatus struct {
	State     State     `json:"state"`
	Type      string    `json:"type"`
	Listen    string    `json:"listen"`
	Protocol  int       `json:"protocol"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error"`
}

type Proxy interface {
	Start() error
	Wait() error
	Stop(ctx context.Context) error
	Status() ProxyStatus
	IPv6Manager() network.Pool
}
