// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import "testing"

func TestSubcommands_SkipsLeadingConfigPairs(t *testing.T) {
	cases := []struct {
		args      []string
		sub, sub2 string
	}{
		{[]string{"worktree", "add", "-b"}, "worktree", "add"},
		{[]string{"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "stash", "show"}, "stash", "show"},
		{[]string{"-c"}, "-c", ""},
		{nil, "", ""},
	}
	for _, c := range cases {
		sub, sub2 := subcommands(c.args)
		if sub != c.sub || sub2 != c.sub2 {
			t.Errorf("subcommands(%q) = (%q, %q), want (%q, %q)", c.args, sub, sub2, c.sub, c.sub2)
		}
	}
}
