// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package skills_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/skills"
)

// TestEmbeddedSkillEqualsSource guards the single source of truth: every
// file under skills/wspace-workspaces on disk must be in the binary's
// embedded copy, byte for byte, and the embedded copy holds nothing else
// (go:embed silently skips names starting with "." or "_").
func TestEmbeddedSkillEqualsSource(t *testing.T) {
	onDisk := map[string][]byte{}
	err := filepath.WalkDir(skills.Name, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		onDisk[filepath.ToSlash(path)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	embedded := map[string][]byte{}
	err = fs.WalkDir(skills.FS(), skills.Name, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(skills.FS(), path)
		if err != nil {
			return err
		}
		embedded[path] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(onDisk) == 0 {
		t.Fatal("no skill files on disk")
	}
	for path, want := range onDisk {
		got, ok := embedded[path]
		if !ok {
			t.Errorf("%s is on disk but not embedded", path)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs between disk and the embedded copy", path)
		}
	}
	for path := range embedded {
		if _, ok := onDisk[path]; !ok {
			t.Errorf("%s is embedded but not on disk", path)
		}
	}
}

var (
	frontmatter = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)
	nameField   = regexp.MustCompile(`(?m)^name: (.+)$`)
	descField   = regexp.MustCompile(`(?m)^description: "(.+)"$`)
	validName   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// TestSkillFrontmatterFollowsTheAgentSkillsSpec checks the spec's
// constraints (agentskills.io/specification): name is 1-64 lowercase
// letters, digits and hyphens and matches the directory; description is
// 1-1024 characters.
func TestSkillFrontmatterFollowsTheAgentSkillsSpec(t *testing.T) {
	data, err := fs.ReadFile(skills.FS(), skills.Name+"/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	fm := frontmatter.FindSubmatch(data)
	if fm == nil {
		t.Fatal("SKILL.md has no YAML frontmatter")
	}
	name := nameField.FindSubmatch(fm[1])
	if name == nil {
		t.Fatal("frontmatter has no name")
	}
	n := strings.TrimSpace(string(name[1]))
	if n != skills.Name || len(n) > 64 || !validName.MatchString(n) {
		t.Fatalf("name %q must equal the directory %q and be 1-64 lowercase letters, digits and hyphens", n, skills.Name)
	}
	desc := descField.FindSubmatch(fm[1])
	if desc == nil {
		t.Fatal("frontmatter has no single-line quoted description")
	}
	if l := len(desc[1]); l < 1 || l > 1024 {
		t.Fatalf("description length %d, want 1-1024", l)
	}
}
