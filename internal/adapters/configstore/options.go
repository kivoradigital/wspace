// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import "github.com/kivoradigital/wspace/internal/domain"

// optionsYAML mirrors domain.Options on disk. It is reused by context
// defaults, per-project options, workspace options and overlay options
// (design.md §5 sample: "copy_env_default" appears identically at every
// layer). Pointer fields preserve ADR D5's "unset vs. zero value"
// distinction across a YAML round trip: an absent key decodes to nil, an
// explicit "false" decodes to a non-nil pointer to false.
type optionsYAML struct {
	BaseBranch        *string  `yaml:"base_branch,omitempty"`
	BranchPrefix      *string  `yaml:"branch_prefix,omitempty"`
	CopyEnvDefault    *bool    `yaml:"copy_env_default,omitempty"`
	FetchBeforeCreate *bool    `yaml:"fetch_before_create,omitempty"`
	Remote            *string  `yaml:"remote,omitempty"`
	EnvPruneDirs      []string `yaml:"env_prune_dirs,omitempty"`
}

func (o optionsYAML) toDomain() domain.Options {
	var baseBranch *domain.BranchName
	if o.BaseBranch != nil {
		bn := domain.BranchName(*o.BaseBranch)
		baseBranch = &bn
	}
	return domain.Options{
		BaseBranch:        baseBranch,
		BranchPrefix:      o.BranchPrefix,
		CopyEnv:           o.CopyEnvDefault,
		FetchBeforeCreate: o.FetchBeforeCreate,
		EnvPruneDirs:      o.EnvPruneDirs,
		Remote:            o.Remote,
	}
}

func fromDomainOptions(o domain.Options) optionsYAML {
	var baseBranch *string
	if o.BaseBranch != nil {
		s := string(*o.BaseBranch)
		baseBranch = &s
	}
	return optionsYAML{
		BaseBranch:        baseBranch,
		BranchPrefix:      o.BranchPrefix,
		CopyEnvDefault:    o.CopyEnv,
		FetchBeforeCreate: o.FetchBeforeCreate,
		Remote:            o.Remote,
		EnvPruneDirs:      o.EnvPruneDirs,
	}
}
