package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const latestReleaseEndpoint = "https://api.github.com/repos/MoonKim-isMe/chzzk-video-downloader/releases/latest"

type Result struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	ReleaseName     string `json:"releaseName"`
	ReleaseURL      string `json:"releaseUrl"`
}

type Checker struct {
	httpClient *http.Client
	endpoint   string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

type semanticVersion struct {
	major int
	minor int
	patch int
}

func NewChecker(httpClient *http.Client) *Checker {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Checker{httpClient: httpClient, endpoint: latestReleaseEndpoint}
}

func Check(ctx context.Context, currentVersion string) (Result, error) {
	return NewChecker(nil).Check(ctx, currentVersion)
}

func (c *Checker) Check(ctx context.Context, currentVersion string) (Result, error) {
	current, err := parseVersion(currentVersion)
	if err != nil {
		return Result{}, fmt.Errorf("현재 앱 버전을 해석할 수 없습니다: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return Result{}, fmt.Errorf("업데이트 확인 요청을 만들 수 없습니다: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "CHZZK-Video-Downloader/"+currentVersion)

	response, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("GitHub Releases를 확인할 수 없습니다: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("GitHub Releases 조회에 실패했습니다 (HTTP %d)", response.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return Result{}, fmt.Errorf("GitHub Release 정보를 읽을 수 없습니다: %w", err)
	}

	latest, err := parseVersion(release.TagName)
	if err != nil {
		return Result{}, fmt.Errorf("최신 Release 버전을 해석할 수 없습니다: %w", err)
	}
	if strings.TrimSpace(release.HTMLURL) == "" {
		return Result{}, fmt.Errorf("최신 Release 페이지 주소가 없습니다")
	}

	return Result{
		UpdateAvailable: latest.greaterThan(current),
		CurrentVersion:  current.String(),
		LatestVersion:   latest.String(),
		ReleaseName:     strings.TrimSpace(release.Name),
		ReleaseURL:      strings.TrimSpace(release.HTMLURL),
	}, nil
}

func parseVersion(value string) (semanticVersion, error) {
	normalized := strings.TrimSpace(value)
	normalized = strings.TrimPrefix(normalized, "release-")
	normalized = strings.TrimPrefix(normalized, "v")

	parts := strings.Split(normalized, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("버전은 major.minor.patch 형식이어야 합니다: %q", value)
	}

	numbers := make([]int, 3)
	for index, part := range parts {
		if part == "" {
			return semanticVersion{}, fmt.Errorf("버전 구성요소가 비어 있습니다: %q", value)
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, fmt.Errorf("버전 구성요소가 숫자가 아닙니다: %q", value)
		}
		numbers[index] = number
	}

	return semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}

func (v semanticVersion) greaterThan(other semanticVersion) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	if v.minor != other.minor {
		return v.minor > other.minor
	}
	return v.patch > other.patch
}

func (v semanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}
