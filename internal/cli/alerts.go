package cli

import (
	"encoding/json"
	"os"
	"os/exec"
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
	if _, err := tmp.Seek(0, 0); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = tmp
	if err := cmd.Start(); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	_ = cmd.Process.Release()
	tmp.Close()
	go func() {
		time.Sleep(5 * time.Second)
		_ = os.Remove(name)
	}()
}
