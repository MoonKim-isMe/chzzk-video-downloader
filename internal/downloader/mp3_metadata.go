package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CHZZK exposes a publication date for VODs. This is not necessarily the
// original live broadcast start date, so never infer a date when it is absent.
var mp3PublicationDatePattern = regexp.MustCompile(`^([0-9]{4})[-./][[:space:]]*([0-9]{1,2})[-./][[:space:]]*([0-9]{1,2})([^0-9]|$)`)

func mp3PublicationDate(value string) string {
	parts := mp3PublicationDatePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(parts) != 5 {
		return ""
	}
	year, errYear := strconv.Atoi(parts[1])
	month, errMonth := strconv.Atoi(parts[2])
	day, errDay := strconv.Atoi(parts[3])
	if errYear != nil || errMonth != nil || errDay != nil {
		return ""
	}
	date := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ""
	}
	return date
}

func mp3MetadataTags(request DownloadRequest) []string {
	title := strings.TrimSpace(request.VideoTitle)
	if request.Segment != nil && strings.TrimSpace(request.Segment.Title) != "" {
		title = strings.TrimSpace(request.Segment.Title)
	}
	tags := make([]string, 0, 10)
	for _, item := range [][2]string{
		{"title", title},
		{"album", strings.TrimSpace(request.VideoTitle)},
		{"artist", strings.TrimSpace(request.ChannelName)},
		{"date", mp3PublicationDate(request.PublishDate)},
	} {
		if item[1] != "" {
			tags = append(tags, "-metadata", item[0]+"="+item[1])
		}
	}
	if request.Segment != nil && request.Segment.Position > 0 {
		tags = append(tags, "-metadata", "track="+strconv.Itoa(request.Segment.Position))
	}
	return tags
}

func buildMP3MetadataCommand(ffmpegPath, inputPath, outputPath string, request DownloadRequest) (CommandSpec, bool) {
	tags := mp3MetadataTags(request)
	if len(tags) == 0 {
		return CommandSpec{}, false
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", inputPath, "-map", "0:a:0", "-c:a", "copy",
	}
	args = append(args, tags...)
	args = append(args, "-id3v2_version", "3", "-f", "mp3", outputPath)
	return CommandSpec{Path: ffmpegPath, Args: args}, true
}

// applyMP3Metadata remuxes only the final MP3 audio stream. It preserves the
// already downloaded file until FFmpeg succeeds, and never re-encodes audio.
func applyMP3Metadata(
	ctx context.Context,
	ffmpegPath string,
	request DownloadRequest,
	finalPath string,
	run func(context.Context, CommandSpec, LineHandler) error,
) error {
	if NormalizeDownloadType(request.DownloadType) != DownloadTypeAudio {
		return nil
	}
	if !strings.EqualFold(filepath.Ext(finalPath), ".mp3") {
		return fmt.Errorf("MP3 결과 파일의 확장자가 올바르지 않습니다")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(mp3MetadataTags(request)) == 0 {
		return nil
	}
	original, err := os.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("MP3 원본 파일을 확인할 수 없습니다: %w", err)
	}
	if !original.Mode().IsRegular() || original.Size() == 0 {
		return fmt.Errorf("MP3 원본 파일이 비어 있거나 일반 파일이 아닙니다")
	}
	file, err := os.CreateTemp(filepath.Dir(finalPath), ".chzzk-mp3-metadata-*.mp3")
	if err != nil {
		return fmt.Errorf("MP3 메타데이터 임시 파일을 만들 수 없습니다: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Close(); err != nil {
		return err
	}
	spec, _ := buildMP3MetadataCommand(ffmpegPath, finalPath, tempPath, request)
	if err := run(ctx, spec, nil); err != nil {
		return fmt.Errorf("MP3 파일 태그 처리에 실패했습니다: %w", err)
	}
	info, err := os.Stat(tempPath)
	if err != nil {
		return fmt.Errorf("메타데이터가 기록된 MP3 결과 파일을 확인할 수 없습니다: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("메타데이터가 기록된 MP3 파일이 비어 있습니다")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, original.Mode().Perm()); err != nil {
		return fmt.Errorf("MP3 파일 권한을 보존하지 못했습니다: %w", err)
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return fmt.Errorf("메타데이터가 기록된 MP3를 최종 파일로 교체하지 못했습니다: %w", err)
	}
	return nil
}
