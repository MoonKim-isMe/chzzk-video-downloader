package downloader

import (
	"errors"
	"strings"
)

type DownloadFailureKind string

const (
	DownloadFailureAuthenticationRequired DownloadFailureKind = "authentication_required"
	DownloadFailurePartialDataConflict     DownloadFailureKind = "partial_data_conflict"
)

type DownloadFailure struct {
	Kind    DownloadFailureKind
	Message string
	Cause   error
}

func (e *DownloadFailure) Error() string {
	return e.Message
}

func (e *DownloadFailure) Unwrap() error {
	return e.Cause
}

func classifyDownloadFailure(err error) error {
	if err == nil {
		return nil
	}

	text := strings.ToLower(err.Error())
	switch {
	case containsAny(
		text,
		"http error 401",
		"401: unauthorized",
		"401 unauthorized",
		"login required",
		"sign in",
		"authentication required",
		"age-restricted",
		"age restricted",
	):
		return &DownloadFailure{
			Kind:    DownloadFailureAuthenticationRequired,
			Message: "로그인이 필요한 콘텐츠입니다. 연령 제한 또는 접근 권한이 필요한 영상일 수 있습니다.",
			Cause:   err,
		}

	case containsAny(
		text,
		"initialization fragment found after media fragments",
		"unable to download",
	) && strings.Contains(text, "initialization fragment"):
		return &DownloadFailure{
			Kind:    DownloadFailurePartialDataConflict,
			Message: "이전 다운로드의 임시 데이터와 충돌했습니다. 임시 파일을 정리한 뒤 다시 시도해 주세요.",
			Cause:   err,
		}
	}

	return err
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func downloadFailureKind(err error) (DownloadFailureKind, bool) {
	var failure *DownloadFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.Kind, true
}
