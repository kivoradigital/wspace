// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package githubrelease implements ports.ReleaseChecker against GitHub's
// releases API (design.md §11). It caches every result to disk so a normal
// invocation almost never touches the network, and it degrades to a cached
// or "unavailable" result instead of ever returning an error for a
// network failure, a rate limit, a timeout, or a malformed response body —
// design.md §11: "Never an error to the user, never a line on stdout".
package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
)

const (
	defaultBaseURL = "https://api.github.com"
	defaultTimeout = 3 * time.Second
	defaultTTL     = 24 * time.Hour
)

// Adapter implements ports.ReleaseChecker. Every field beyond CachePath has
// a working default from New and exists to be overridden by a test: BaseURL
// for an httptest.Server, TTL to avoid a real test waiting 24h, Now to
// control cache-age comparisons deterministically.
type Adapter struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
	CachePath  domain.Path
	TTL        time.Duration
	Now        func() time.Time
}

// New constructs an Adapter caching to cachePath (the composition root
// passes fsstore's own Paths().Cache.Join("release.json") — never a
// hardcoded path, per this phase's own constraint).
func New(cachePath domain.Path) *Adapter {
	return &Adapter{
		BaseURL:    defaultBaseURL,
		UserAgent:  "ws",
		HTTPClient: &http.Client{Timeout: defaultTimeout},
		CachePath:  cachePath,
		TTL:        defaultTTL,
		Now:        time.Now,
	}
}

// cacheData is release.json's on-disk shape (design.md §11: "{tag, url,
// etag, checked_at}").
type cacheData struct {
	Tag       string    `json:"tag"`
	URL       string    `json:"url"`
	ETag      string    `json:"etag"`
	CheckedAt time.Time `json:"checked_at"`
}

// releaseBody is the subset of GitHub's release JSON this adapter reads.
type releaseBody struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Latest implements ports.ReleaseChecker. It never returns a non-nil error
// for a degraded network condition — only a genuinely unexpected local
// failure (building the HTTP request itself) can produce one, and even
// that is not exercised by any real caller path since the request is
// always well-formed.
func (a *Adapter) Latest(ctx context.Context, c domain.RepoCoordinates) (domain.ReleaseInfo, error) {
	cached, hasCache := a.loadCache()

	if hasCache && a.Now().Sub(cached.CheckedAt) < a.TTL {
		return domain.ReleaseInfo{Tag: cached.Tag, URL: cached.URL}, nil
	}

	fresh, status, err := a.fetch(ctx, c, cached, hasCache)
	if degraded, ok := a.classifyDegradation(status, err, fresh, cached, hasCache); ok {
		return degraded, nil
	}

	if status == http.StatusNotModified {
		cached.CheckedAt = a.Now()
		_ = a.saveCache(cached)
		return domain.ReleaseInfo{Tag: cached.Tag, URL: cached.URL}, nil
	}

	fresh.CheckedAt = a.Now()
	_ = a.saveCache(fresh)
	return domain.ReleaseInfo{Tag: fresh.Tag, URL: fresh.URL}, nil
}

// classifyDegradation is the one shared helper (tasks.md 5.11's REFACTOR)
// every non-happy-path outcome funnels through: a transport error (offline,
// timeout, DNS failure), a rate-limited response (403 or 429), any other
// unexpected status, and a 200 whose body did not decode into a usable
// tag_name all degrade identically — to the cached value marked Stale when
// one exists, or to Unavailable when it does not. ok is false only for the
// two outcomes that need their own handling in Latest: a genuine 304
// (revalidated, not degraded) and a genuine fresh 200 with a usable tag.
func (a *Adapter) classifyDegradation(status int, err error, fresh cacheData, cached cacheData, hasCache bool) (domain.ReleaseInfo, bool) {
	degraded := err != nil ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		(status != http.StatusOK && status != http.StatusNotModified) ||
		(status == http.StatusOK && fresh.Tag == "")

	if !degraded {
		return domain.ReleaseInfo{}, false
	}
	if hasCache {
		return domain.ReleaseInfo{Tag: cached.Tag, URL: cached.URL, Stale: true}, true
	}
	return domain.ReleaseInfo{Unavailable: true}, true
}

// fetch performs the actual GitHub request. Any transport-level error
// (offline, DNS failure, deadline exceeded) is returned as err with a zero
// status, which classifyDegradation treats identically to a rate limit.
func (a *Adapter) fetch(ctx context.Context, c domain.RepoCoordinates, cached cacheData, hasCache bool) (cacheData, int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", a.BaseURL, c.Owner, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return cacheData{}, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if a.UserAgent != "" {
		req.Header.Set("User-Agent", a.UserAgent)
	}
	if hasCache && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return cacheData{}, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return cacheData{}, resp.StatusCode, nil
	}

	var body releaseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return cacheData{}, resp.StatusCode, nil
	}
	return cacheData{Tag: body.TagName, URL: body.HTMLURL, ETag: resp.Header.Get("ETag")}, resp.StatusCode, nil
}

// loadCache reads CachePath. A missing file, unreadable file, or one
// without a usable Tag is reported as "no cache" rather than an error —
// this adapter must never fail a caller over its own cache file.
func (a *Adapter) loadCache() (cacheData, bool) {
	raw, err := os.ReadFile(string(a.CachePath))
	if err != nil {
		return cacheData{}, false
	}
	var c cacheData
	if err := json.Unmarshal(raw, &c); err != nil || c.Tag == "" {
		return cacheData{}, false
	}
	return c, true
}

// saveCache writes c to CachePath, creating its parent directory if
// needed. A write failure is swallowed by every caller (Latest never fails
// a command over an unwritable cache directory) but returned here so a
// future caller that does care can still observe it.
func (a *Adapter) saveCache(c cacheData) error {
	dir := filepath.Dir(string(a.CachePath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(string(a.CachePath), data, 0o644)
}
