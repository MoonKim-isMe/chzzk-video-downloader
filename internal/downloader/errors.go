package downloader

import (
	"errors"
	"strings"
)

type DownloadFailureKind string

const (
	DownloadFailureAuthenticationRequired       DownloadFailureKind = "authentication_required"
	DownloadFailurePartialDataConflict           DownloadFailureKind = "partial_data_conflict"
	DownloadFailureHLSInitializationFragmentOrder DownloadFailureKind = "hls_initialization_fragment_order"
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
			Message: "로그인이 필요한 콘텐츠입니다. 상단의 인증에서 로그인 정보와 해당 계정의 접근 권한을 확인해 주세요.",
			Cause:   err,
		}

	case strings.Contains(text, "initialization fragment found after media fragments"):
		return &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조를 일반 방식으로 처리할 수 없어 대체 다운로드 방식으로 다시 시도합니다.",
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

func isDownloadFailureKind(err error, kind DownloadFailureKind) bool {
	current, ok := downloadFailureKind(err)
	return ok && current == kind
}
