package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jannik-schroeder/nyxproxy-oss/internal/app"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/config"
	"github.com/jannik-schroeder/nyxproxy-oss/internal/logbuf"
	"github.com/jannik-schroeder/nyxproxy-oss/pkg/network"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, session *Session) {
	writeJSON(w, http.StatusOK, s.opts.Controller.Status())
}

func (s *Server) handlePool(w http.ResponseWriter, r *http.Request, session *Session) {
	writeJSON(w, http.StatusOK, s.opts.Controller.PoolDetails())
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request, session *Session) {
	cfg, generation := s.opts.Controller.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"config": cfg.Redacted(),
		"meta": map[string]any{
			"path":          s.opts.Store.Path(),
			"generation":    generation,
			"locked":        config.LockedFields(),
			"passwords_set": cfg.PasswordsSet(),
		},
	})
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request, session *Session) {
	var submitted config.ProxyConfig
	if derr := decodeJSON(w, r, &submitted); derr != nil {
		writeError(w, derr.status, derr.code, derr.message, nil)
		return
	}

	result, err := s.opts.Controller.Apply(r.Context(), submitted)
	if err != nil {
		var applyErr *app.ApplyError
		switch {
		case errors.Is(err, app.ErrApplyInProgress):
			writeError(w, http.StatusConflict, "apply_in_progress", "a configuration apply is already in progress", nil)
		case errors.As(err, &applyErr):
			status := http.StatusInternalServerError
			switch applyErr.Code {
			case "validation_failed", "field_locked", "restart_required":
				status = http.StatusUnprocessableEntity
			case "apply_in_progress":
				status = http.StatusConflict
			}
			var rolledBack *bool
			if applyErr.Code == "apply_failed" || applyErr.Code == "persist_failed" {
				rolledBack = &applyErr.RolledBack
			}
			writeJSON(w, status, errorEnvelope{Error: apiError{
				Code:       applyErr.Code,
				Message:    applyErr.Message,
				Fields:     applyErr.Fields,
				RolledBack: rolledBack,
			}})
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		}
		return
	}

	if result.ReauthRequired {
		cfg, _ := s.opts.Controller.Config()
		s.auth.SetCredentials(cfg.Web.Username, cfg.Web.Password)
		s.auth.ClearCookie(w, r)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request, session *Session) {
	err := s.opts.Controller.Retry(r.Context())
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, app.ErrApplyInProgress):
		writeError(w, http.StatusConflict, "apply_in_progress", "a configuration apply is already in progress", nil)
	case errors.Is(err, app.ErrProxyAlreadyRunning):
		writeError(w, http.StatusConflict, "proxy_already_running", "proxy is already running", nil)
	default:
		writeError(w, http.StatusInternalServerError, "apply_failed", err.Error(), nil)
	}
}

func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request, session *Session) {
	details, err := network.ListInterfaceDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request, session *Session) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming is not supported", nil)
		return
	}
	if s.opts.Logs == nil {
		writeError(w, http.StatusServiceUnavailable, "internal_error", "log stream unavailable", nil)
		return
	}

	ch, cancel, ok := s.opts.Logs.Subscribe()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "internal_error", "too many log streams", nil)
		return
	}
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var after uint64
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if value, err := strconv.ParseUint(raw, 10, 64); err == nil {
			after = value
		}
	}
	if after > s.opts.Logs.LastSeq() {
		// The ID belongs to a previous process; sequence numbers restarted at 1.
		after = 0
	}

	records, dropped := s.opts.Logs.Replay(after)
	lastSeq := after
	if dropped > 0 {
		fmt.Fprintf(w, "event: gap\ndata: {\"dropped\":%d}\n\n", dropped)
	}
	for _, record := range records {
		writeLogEvent(w, record)
		lastSeq = record.Seq
	}
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	maxLifetime := time.NewTimer(30 * time.Minute)
	defer maxLifetime.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.stopStreams:
			return
		case <-maxLifetime.C:
			return
		case <-heartbeat.C:
			if s.auth.SessionByID(session.ID) == nil {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case record, open := <-ch:
			if !open {
				return
			}
			if record.Seq <= lastSeq {
				continue
			}
			writeLogEvent(w, record)
			lastSeq = record.Seq
			flusher.Flush()
		}
	}
}

func writeLogEvent(w http.ResponseWriter, record logbuf.Record) {
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: log\nid: %d\ndata: %s\n\n", record.Seq, data)
}
