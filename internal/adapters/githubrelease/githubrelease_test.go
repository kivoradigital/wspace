// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package githubrelease_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/githubrelease"
	"github.com/kivoradigital/wspace/internal/domain"
)

func cachePath(t *testing.T) domain.Path {
	t.Helper()
	return domain.Path(filepath.Join(t.TempDir(), "release.json"))
}

func writeCacheFile(t *testing.T, path domain.Path, tag, url, etag string, checkedAt time.Time) {
	t.Helper()
	data, err := json.Marshal(struct {
		Tag       string    `json:"tag"`
		URL       string    `json:"url"`
		ETag      string    `json:"etag"`
		CheckedAt time.Time `json:"checked_at"`
	}{Tag: tag, URL: url, ETag: etag, CheckedAt: checkedAt})
	if err != nil {
		t.Fatalf("marshal seed cache: %v", err)
	}
	if err := os.WriteFile(string(path), data, 0o644); err != nil {
		t.Fatalf("write seed cache: %v", err)
	}
}

// TestReleaseChecker_200_CachesETagAndTag covers tasks.md 5.5: a fresh 200
// response is cached, and a second call within the TTL reuses the cache
// instead of issuing a new request (update-check spec: "Result caching").
func TestReleaseChecker_200_CachesETagAndTag(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/repos/acme/widget/releases/latest" {
			t.Errorf("request path = %q, want /repos/acme/widget/releases/latest", r.URL.Path)
		}
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://example.invalid/v1.2.3"}`))
	}))
	defer srv.Close()

	adapter := githubrelease.New(cachePath(t))
	adapter.BaseURL = srv.URL

	got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
	if err != nil {
		t.Fatalf("Latest() unexpected error: %v", err)
	}
	if got.Tag != "v1.2.3" || got.URL != "https://example.invalid/v1.2.3" || got.Stale || got.Unavailable {
		t.Fatalf("Latest() = %+v, want Tag=v1.2.3, URL set, Stale=false, Unavailable=false", got)
	}
	if hits != 1 {
		t.Fatalf("server hits after first call = %d, want 1", hits)
	}

	got2, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
	if err != nil {
		t.Fatalf("second Latest() unexpected error: %v", err)
	}
	if got2 != got {
		t.Fatalf("second Latest() = %+v, want identical cached %+v", got2, got)
	}
	if hits != 1 {
		t.Fatalf("server hits after second call (within TTL) = %d, want still 1 (cache reused)", hits)
	}
}

// TestReleaseChecker_304_RefreshesCheckedAtOnly covers tasks.md 5.6: past
// TTL, revalidation is sent with If-None-Match; a 304 only refreshes
// checked_at, never changing the cached tag/url.
func TestReleaseChecker_304_RefreshesCheckedAtOnly(t *testing.T) {
	var gotIfNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	path := cachePath(t)
	writeCacheFile(t, path, "v1.0.0", "https://example.invalid/v1.0.0", `"etag-1"`, time.Now().Add(-48*time.Hour))

	adapter := githubrelease.New(path)
	adapter.BaseURL = srv.URL
	adapter.TTL = 24 * time.Hour
	fixedNow := time.Now()
	adapter.Now = func() time.Time { return fixedNow }

	got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
	if err != nil {
		t.Fatalf("Latest() unexpected error: %v", err)
	}
	if got.Tag != "v1.0.0" || got.URL != "https://example.invalid/v1.0.0" || got.Stale || got.Unavailable {
		t.Fatalf("Latest() = %+v, want the unchanged cached tag/url with Stale=false", got)
	}
	if gotIfNoneMatch != `"etag-1"` {
		t.Fatalf("If-None-Match = %q, want the cached etag", gotIfNoneMatch)
	}

	raw, err := os.ReadFile(string(path))
	if err != nil {
		t.Fatalf("read cache file after 304: %v", err)
	}
	var onDisk struct {
		Tag       string    `json:"tag"`
		CheckedAt time.Time `json:"checked_at"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal cache file: %v", err)
	}
	if onDisk.Tag != "v1.0.0" {
		t.Fatalf("cache tag on disk = %q, want unchanged v1.0.0", onDisk.Tag)
	}
	if !onDisk.CheckedAt.Equal(fixedNow) {
		t.Fatalf("cache checked_at on disk = %v, want refreshed to %v", onDisk.CheckedAt, fixedNow)
	}
}

// TestReleaseChecker_403And429_FallBackToStaleCache covers tasks.md 5.7:
// rate-limiting (403 or 429) degrades to the cached value marked Stale when
// a cache exists, and to Unavailable when it does not — never a Go error
// (update-check spec: "Graceful offline degradation").
func TestReleaseChecker_403And429_FallBackToStaleCache(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			path := cachePath(t)
			writeCacheFile(t, path, "v1.0.0", "https://example.invalid/v1.0.0", "", time.Now().Add(-48*time.Hour))

			adapter := githubrelease.New(path)
			adapter.BaseURL = srv.URL
			adapter.TTL = 24 * time.Hour

			got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
			if err != nil {
				t.Fatalf("Latest() unexpected error: %v", err)
			}
			if got.Tag != "v1.0.0" || !got.Stale || got.Unavailable {
				t.Fatalf("Latest() = %+v, want the stale cached tag with Stale=true, Unavailable=false", got)
			}
		})
	}
}

// TestReleaseChecker_403And429_NoCacheReportsUnavailable is the other half
// of 5.7: without any prior cache, the same degraded statuses report
// Unavailable instead of a fabricated tag.
func TestReleaseChecker_403And429_NoCacheReportsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	adapter := githubrelease.New(cachePath(t))
	adapter.BaseURL = srv.URL

	got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
	if err != nil {
		t.Fatalf("Latest() unexpected error: %v", err)
	}
	if !got.Unavailable {
		t.Fatalf("Latest() = %+v, want Unavailable=true with no cache", got)
	}
}

// TestReleaseChecker_Timeout_NeverErrorsToUser covers tasks.md 5.8: a
// deadline exceeded against a slow server never surfaces as a Go error —
// it degrades exactly like an offline network, per the interface's own
// contract ("callers pass a short-deadline context").
func TestReleaseChecker_Timeout_NeverErrorsToUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter := githubrelease.New(cachePath(t))
	adapter.BaseURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	got, err := adapter.Latest(ctx, domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
	if err != nil {
		t.Fatalf("Latest() returned an error on timeout, want nil (never an error to the user): %v", err)
	}
	if !got.Unavailable {
		t.Fatalf("Latest() = %+v, want Unavailable=true with no cache and a timed-out request", got)
	}
}

// TestReleaseChecker_MalformedBody_ReturnsCachedOrUnavailable covers tasks.md
// 5.9: a 200 response whose body cannot be parsed (or carries no tag_name)
// degrades exactly like a network failure.
func TestReleaseChecker_MalformedBody_ReturnsCachedOrUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	t.Run("no cache reports unavailable", func(t *testing.T) {
		adapter := githubrelease.New(cachePath(t))
		adapter.BaseURL = srv.URL

		got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
		if err != nil {
			t.Fatalf("Latest() unexpected error: %v", err)
		}
		if !got.Unavailable {
			t.Fatalf("Latest() = %+v, want Unavailable=true for a malformed body with no cache", got)
		}
	})

	t.Run("existing cache falls back stale", func(t *testing.T) {
		path := cachePath(t)
		writeCacheFile(t, path, "v1.0.0", "https://example.invalid/v1.0.0", "", time.Now().Add(-48*time.Hour))

		adapter := githubrelease.New(path)
		adapter.BaseURL = srv.URL
		adapter.TTL = 24 * time.Hour

		got, err := adapter.Latest(context.Background(), domain.RepoCoordinates{Owner: "acme", Repo: "widget"})
		if err != nil {
			t.Fatalf("Latest() unexpected error: %v", err)
		}
		if got.Tag != "v1.0.0" || !got.Stale {
			t.Fatalf("Latest() = %+v, want the stale cached tag for a malformed body", got)
		}
	})
}
