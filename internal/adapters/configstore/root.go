// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"gopkg.in/yaml.v3"
)

// rootConfigYAML mirrors "<config>/ws/config.yaml" (design.md §5).
type rootConfigYAML struct {
	SchemaVersion int             `yaml:"schema_version"`
	ActiveContext string          `yaml:"active_context"`
	Preferences   preferencesYAML `yaml:"preferences"`
}

// preferencesYAML mirrors domain.Preferences; durations are encoded as
// Go duration strings ("24h", "60s") per the design.md §5 sample.
type preferencesYAML struct {
	Locale            string `yaml:"locale"`
	UpdateCheck       bool   `yaml:"update_check"`
	UpdateCheckTTL    string `yaml:"update_check_ttl,omitempty"`
	TrayRefreshPeriod string `yaml:"tray_refresh_period,omitempty"`
}

func (p preferencesYAML) toDomain() (domain.Preferences, error) {
	ttl, err := parseDuration(p.UpdateCheckTTL)
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("config.load: update_check_ttl: %w", err)
	}
	refresh, err := parseDuration(p.TrayRefreshPeriod)
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("config.load: tray_refresh_period: %w", err)
	}
	return domain.Preferences{
		Locale:            p.Locale,
		UpdateCheck:       p.UpdateCheck,
		UpdateCheckTTL:    ttl,
		TrayRefreshPeriod: refresh,
	}, nil
}

func fromDomainPreferences(p domain.Preferences) preferencesYAML {
	return preferencesYAML{
		Locale:            p.Locale,
		UpdateCheck:       p.UpdateCheck,
		UpdateCheckTTL:    formatDuration(p.UpdateCheckTTL),
		TrayRefreshPeriod: formatDuration(p.TrayRefreshPeriod),
	}
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func (a *Adapter) rootConfigPath() string {
	return filepath.Join(filepath.FromSlash(string(a.root)), "config.yaml")
}

// LoadRoot reads "<root>/config.yaml". A missing file reports
// ports.ErrConfigNotInitialized cleanly (design.md §5 "First run and
// migration") rather than a raw os.PathError.
func (a *Adapter) LoadRoot(_ context.Context) (domain.RootConfig, error) {
	data, err := os.ReadFile(a.rootConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return domain.RootConfig{}, ports.ErrConfigNotInitialized
		}
		return domain.RootConfig{}, fmt.Errorf("config.load: %w", err)
	}

	var dto rootConfigYAML
	if err := strictDecode(data, &dto); err != nil {
		return domain.RootConfig{}, fmt.Errorf("config.load: %w", err)
	}
	if err := checkSchemaVersion("config.load", &dto.SchemaVersion); err != nil {
		return domain.RootConfig{}, err
	}

	prefs, err := dto.Preferences.toDomain()
	if err != nil {
		return domain.RootConfig{}, err
	}

	name, err := domain.NewContextName(dto.ActiveContext)
	if err != nil && dto.ActiveContext != "" {
		return domain.RootConfig{}, fmt.Errorf("config.load: active_context: %w", err)
	}
	if dto.ActiveContext == "" {
		name = ""
	}

	return domain.RootConfig{
		SchemaVersion: dto.SchemaVersion,
		ActiveContext: name,
		Preferences:   prefs,
	}, nil
}

// SaveRoot atomically writes r to "<root>/config.yaml" (design.md §5:
// "Writes are atomic").
func (a *Adapter) SaveRoot(_ context.Context, r domain.RootConfig) error {
	version := r.SchemaVersion
	if version == 0 {
		version = currentSchemaVersion
	}
	dto := rootConfigYAML{
		SchemaVersion: version,
		ActiveContext: string(r.ActiveContext),
		Preferences:   fromDomainPreferences(r.Preferences),
	}
	data, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("config.save: %w", err)
	}
	if err := atomicWrite(a.rootConfigPath(), data); err != nil {
		return fmt.Errorf("config.save: %w", err)
	}
	return nil
}
