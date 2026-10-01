package downloader

import (
	"fmt"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func ApplySettings(request StartDownloadRequest, value appsettings.AppSettings) (StartDownloadRequest, error) {
	value = appsettings.Normalize(value)
	if err := appsettings.Validate(value); err != nil {
		return StartDownloadRequest{}, err
	}

	formatSelector, err := FormatSelectorForResolution(value.Resolution)
	if err != nil {
		return StartDownloadRequest{}, err
	}
	concurrentFragments, err := ConcurrentFragmentsForAcceleration(value.DownloadAcceleration)
	if err != nil {
		return StartDownloadRequest{}, err
	}

	request.OutputDir = value.DownloadDir
	request.FormatSelector = formatSelector
	request.OutputFormat = string(value.OutputFormat)
	request.ConcurrentFragments = concurrentFragments
	return request, nil
}

func ConcurrentFragmentsForAcceleration(acceleration appsettings.DownloadAcceleration) (int, error) {
	switch acceleration {
	case appsettings.DownloadAccelerationStable:
		return 1, nil
	case appsettings.DownloadAccelerationStandard:
		return 2, nil
	case appsettings.DownloadAccelerationFast:
		return 4, nil
	case appsettings.DownloadAccelerationUltra:
		return 8, nil
	default:
		return 0, fmt.Errorf("지원하지 않는 다운로드 가속 설정입니다: %s", acceleration)
	}
}

func FormatSelectorForResolution(resolution appsettings.Resolution) (string, error) {
	switch resolution {
	case appsettings.ResolutionBest:
		return DefaultFormatSelector, nil
	case appsettings.Resolution2160p:
		return formatSelectorForMaxHeight(2160), nil
	case appsettings.Resolution1440p:
		return formatSelectorForMaxHeight(1440), nil
	case appsettings.Resolution1080p:
		return formatSelectorForMaxHeight(1080), nil
	case appsettings.Resolution720p:
		return formatSelectorForMaxHeight(720), nil
	default:
		return "", fmt.Errorf("지원하지 않는 해상도입니다: %s", resolution)
	}
}

func formatSelectorForMaxHeight(height int) string {
	return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]", height, height)
}
