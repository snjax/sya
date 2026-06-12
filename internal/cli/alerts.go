package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/snjax/sya/internal/doctor"
)

type doctorAlertEvent struct {
	TS      time.Time `json:"ts"`
	Actor   string    `json:"actor,omitempty"`
	Finding any       `json:"finding"`
}

func (a *App) fireDeniedTransitionAlert(project Project, event any) {
	cfg, err := loadConfig(project)
	if err != nil || cfg.Alerts.DeniedTransition == "" {
		return
	}
	a.startAlert(cfg.Alerts.DeniedTransition, event)
}

func (a *App) fireDoctorViolationAlerts(project Project, findings []doctor.Finding) {
	if len(findings) == 0 {
		return
	}
	cfg, err := loadConfig(project)
	if err != nil || cfg.Alerts.DoctorViolation == "" {
		return
	}
	for _, finding := range findings {
		a.startAlert(cfg.Alerts.DoctorViolation, doctorAlertEvent{
			TS:      a.now().UTC(),
			Actor:   a.Actor(),
			Finding: finding,
		})
	}
}

func (a *App) startAlert(command string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp("", "sya-alert-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return
	}
	script := command + " < " + shellQuoteAlert(name) + "; rm -f " + shellQuoteAlert(name)
	cmd := exec.Command("sh", "-c", script)
	waitForExit := false
	if _, err := exec.LookPath("timeout"); err == nil {
		cmd = exec.Command("timeout", "5", "sh", "-c", script)
		waitForExit = true
	}
	if err := cmd.Start(); err != nil {
		os.Remove(name)
		return
	}
	if !waitForExit {
		_ = cmd.Process.Release()
		go func() {
			time.Sleep(1 * time.Second)
			_ = os.Remove(name)
		}()
		return
	}
	go func() {
		_ = cmd.Wait()
		_ = os.Remove(name)
	}()
}

func shellQuoteAlert(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
