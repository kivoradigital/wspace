// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/messages"
)

// wizardFieldLabelKeys is every message.Key used in production code as a
// ports.Field's Label — never its Help, which is deliberately allowed to
// run long since it renders as its own wrapped caption, never the shared
// label column a widget.Form sizes to its widest entry (see
// internal/adapters/formprompt's formItem). This list is maintained by
// hand rather than derived by reflection: adding a new wizard field means
// adding its Label key here too, which is the point of this test — a
// future label with an explanation crammed back into it fails the build
// here, rather than silently reproducing the "one field, a window roughly
// 2000 points wide" defect this change's own label/Help split fixes.
var wizardFieldLabelKeys = []messages.Key{
	messages.WizardContextName,
	messages.WizardWorkspacesRoot,
	messages.WizardProjectsRoot,
	messages.WizardBaseBranch,
	messages.WizardCopyEnvDefault,
	messages.WizardFetchBeforeCreate,
	messages.WizardIgnorePatterns,
	messages.WizardProjectKey,
	messages.WizardAddProjectManually,
	messages.WizardProjectSourceDir,
	messages.WizardProjectOriginBranch,
	messages.WizardProjectDestBranch,
	messages.WizardProjectWorktreeDir,
	messages.WizardContextToDelete,
	messages.WizardImportSourceChoice,
	messages.WizardPickProjects,
	messages.WizardSelectAll,
	messages.WizardWorkspaceName,
	messages.WizardWorkspaceBranch,
	messages.WizardWorkspaceProjects,
	messages.InstallPathConfirm,
	messages.InstallShellRCConfirm,
}

// maxWizardFieldLabelRunes is generous enough for a short noun phrase
// ("Destination branch template" is 28 runes) but far short of the
// paragraph-length labels the product owner's screenshots traced two
// defects to: a one-field step rendering a comically tiny window, and a
// step whose longest label was a full sentence rendering a window roughly
// 2000 points wide.
const maxWizardFieldLabelRunes = 40

// TestWizardFieldLabels_StayShort turns the "long label wrecks the form
// layout" defect into a build-time failure instead of a visual one: every
// known field label must render under maxWizardFieldLabelRunes runes. A
// field whose meaning genuinely needs more than a short noun phrase gets a
// Help key instead (see the *Help keys' own doc comment in keys.go).
func TestWizardFieldLabels_StayShort(t *testing.T) {
	if err := messages.Use("en"); err != nil {
		t.Fatalf("messages.Use(en): %v", err)
	}
	for _, key := range wizardFieldLabelKeys {
		text := messages.T(key)
		if n := len([]rune(text)); n > maxWizardFieldLabelRunes {
			t.Errorf("label %s = %q (%d runes), want <= %d runes — move the explanation into the field's Help key instead", key, text, n, maxWizardFieldLabelRunes)
		}
	}
}

// hasFormatVerb reports whether s contains a '%' that is not part of a
// literal, escaped "%%" — i.e. a genuine fmt format verb. Checked
// directly against the raw template rather than through fmt.Sprintf
// itself (which would need a dynamic, argument-less format string
// staticcheck rightly flags as suspicious on its own terms) — this is a
// plainer, dependency-free version of the exact same question.
func hasFormatVerb(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++ // an escaped "%%" is a literal percent sign, not a verb.
			continue
		}
		return true
	}
	return false
}

// TestWizardFieldLabels_NoFormatVerbs guards the other half of the
// label/Help split: every field's Label template must carry no format
// verb at all, since both prompter adapters (TerminalPrompter.printLabel,
// formprompt.formItem) apply a field's Args exclusively to Help, never to
// Label — see their own doc comments for the production screenshot this
// fixed ("Worktree directory template%!(EXTRA string=...)"). A label
// re-gaining a "%[1]s" placeholder would render that same literal
// "%!(EXTRA ...)" noise the moment any caller sets Args, so this is caught
// here, at the catalog itself, rather than only in a specific prompter's
// own test.
func TestWizardFieldLabels_NoFormatVerbs(t *testing.T) {
	if err := messages.Use("en"); err != nil {
		t.Fatalf("messages.Use(en): %v", err)
	}
	entries := messages.EnglishEntriesForTest()
	for _, key := range wizardFieldLabelKeys {
		tmpl, ok := entries[key]
		if !ok {
			t.Fatalf("label %s has no catalog entry", key)
		}
		if hasFormatVerb(tmpl) {
			t.Errorf("label %s = %q contains a format verb — Args is applied to a field's Help only, never its Label; move any placeholder into the paired Help key instead", key, tmpl)
		}
	}
}
