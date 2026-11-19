package downloader

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The browser can download TS media directly from an MPD SegmentTemplate
// even when the advertised HLS .m3u8 URL is unavailable. Produce an ordinary
// local HLS playlist so the existing FFmpeg proxy, cache, retries and speed
// settings can be reused without exposing the CDN credential to FFmpeg.
const sourceURLMPDMaxSegments = 20000

var sourceURLMPDNumber = regexp.MustCompile(`\$Number(?:%0([1-9][0-9]?)d)?\$`)
var sourceURLMPDRepresentationID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type sourceURLMPDPlaylist struct {
	Periods []struct {
		Adaptations []struct {
			Representations []struct {
				ID      string     `xml:"id,attr"`
				BaseURL string     `xml:"BaseURL"`
				Attrs   []xml.Attr `xml:",any,attr"`
				Template struct {
					Media       string `xml:"media,attr"`
					StartNumber int64  `xml:"startNumber,attr"`
					Timescale   int64  `xml:"timescale,attr"`
					Timeline struct {
						Segments []struct {
							Duration int64 `xml:"d,attr"`
							Repeat   int64 `xml:"r,attr"`
						} `xml:"S"`
					} `xml:"SegmentTimeline"`
				} `xml:"SegmentTemplate"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

// A generated HLS playlist is intentionally limited in size and segment count.
// It accepts only DASH number-based TS templates already tied to the selected
// signed NAVER playlist; Time-based or open-ended templates remain unsupported.
func buildSourceURLMPDHLSPlaylist(mpd []byte, selectedURL string) ([]byte, error) {
	selected, err := url.Parse(selectedURL)
	if err != nil || !validSourceURLHLSAddress(selectedURL) ||
		!isSourceURLAkamaiHost(selected.Hostname()) ||
		sourceURLHLSAuthQueryKey(selected) != "__gda__" {
		return nil, fmt.Errorf("지원하지 않는 MPD HLS 주소입니다")
	}
	var manifest sourceURLMPDPlaylist
	if err := xml.Unmarshal(mpd, &manifest); err != nil {
		return nil, fmt.Errorf("MPD 구간 정보를 해석할 수 없습니다")
	}
	for _, period := range manifest.Periods {
		for _, adaptation := range period.Adaptations {
			for _, rep := range adaptation.Representations {
				matched := false
				for _, attr := range rep.Attrs {
					if attr.Name.Space == "urn:naver:vod:2020" && attr.Name.Local == "m3u" && attr.Value == selectedURL {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
				if !sourceURLMPDRepresentationID.MatchString(rep.ID) {
					return nil, fmt.Errorf("MPD 구간 식별자가 올바르지 않습니다")
				}
				template := rep.Template
				if template.StartNumber < 0 || template.Timescale <= 0 ||
					!strings.Contains(template.Media, "$RepresentationID$") ||
					sourceURLMPDNumber.FindString(template.Media) == "" ||
					len(template.Timeline.Segments) == 0 {
					return nil, fmt.Errorf("지원하지 않는 MPD 구간 형식입니다")
				}
				parent := selected
				if rep.BaseURL != "" {
					parentBase, parseErr := url.Parse(strings.TrimSpace(rep.BaseURL))
					if parseErr != nil || !validSignedHLSResource(parentBase) ||
						!strings.EqualFold(parentBase.Hostname(), selected.Hostname()) ||
						!strings.HasSuffix(parentBase.Path, "/") {
						return nil, fmt.Errorf("허용되지 않은 MPD 구간 기준 주소입니다")
					}
					clone := *parentBase
					query := clone.Query()
					query.Set("__gda__", selected.Query().Get("__gda__"))
					clone.RawQuery = query.Encode()
					parent = &clone
				}
				segmentCount := int64(0)
				targetDuration := int64(1)
				for _, item := range template.Timeline.Segments {
					if item.Duration <= 0 || item.Repeat < 0 ||
						item.Repeat >= sourceURLMPDMaxSegments ||
						segmentCount > sourceURLMPDMaxSegments-1-item.Repeat {
						return nil, fmt.Errorf("MPD 구간 개수가 허용 범위를 초과했습니다")
					}
					segmentCount += item.Repeat + 1
					ceil := item.Duration / template.Timescale
					if item.Duration%template.Timescale != 0 {
						ceil++
					}
					if ceil > targetDuration {
						targetDuration = ceil
					}
				}
				if segmentCount == 0 || segmentCount > sourceURLMPDMaxSegments ||
					rep.Template.StartNumber > math.MaxInt64-segmentCount {
					return nil, fmt.Errorf("MPD 구간 번호가 허용 범위를 초과했습니다")
				}
				var playlist strings.Builder
				fmt.Fprintf(&playlist, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", targetDuration)
				number := template.StartNumber
				for _, item := range template.Timeline.Segments {
					for i := int64(0); i <= item.Repeat; i++ {
						media, subErr := sourceURLMPDMediaName(template.Media, rep.ID, number)
						if subErr != nil {
							return nil, subErr
						}
						resource, resolveErr := signedHLSChildURL(parent, media)
						if resolveErr != nil || sourceURLHLSAuthQueryKey(resource) != "__gda__" ||
							!strings.EqualFold(filepath.Ext(resource.Path), ".ts") {
							return nil, fmt.Errorf("MPD 영상 조각의 인증 주소가 올바르지 않습니다")
						}
						// The generated manifest must remain compatible with the
						// bounded downloader playlist parser and temporary storage.
						if playlist.Len()+len(resource.String())+64 > maxSignedHLSPlaylistBytes {
							return nil, fmt.Errorf("MPD 재생목록 크기가 허용 범위를 초과했습니다")
						}
						fmt.Fprintf(&playlist, "#EXTINF:%.3f,\n%s\n", float64(item.Duration)/float64(template.Timescale), resource.String())
						number++
					}
				}
				playlist.WriteString("#EXT-X-ENDLIST\n")
				return []byte(playlist.String()), nil
			}
		}
	}
	return nil, fmt.Errorf("선택한 HLS 화질의 MPD 구간 정보가 없습니다")
}

func sourceURLMPDMediaName(media, repID string, number int64) (string, error) {
	found := sourceURLMPDNumber.FindStringSubmatch(media)
	if len(found) == 0 {
		return "", fmt.Errorf("MPD 구간 번호 형식이 올바르지 않습니다")
	}
	formatted := strconv.FormatInt(number, 10)
	if found[1] != "" {
		width, err := strconv.Atoi(found[1])
		if err != nil || width > 12 {
			return "", fmt.Errorf("지원하지 않는 MPD 구간 번호 너비입니다")
		}
		formatted = fmt.Sprintf("%0*d", width, number)
	}
	name := strings.ReplaceAll(media, "$RepresentationID$", repID)
	name = strings.ReplaceAll(name, found[0], formatted)
	if strings.Contains(name, "$") {
		return "", fmt.Errorf("지원하지 않는 MPD 구간 변수입니다")
	}
	return name, nil
}

func prepareSourceURLHLSPlaylist(ctx context.Context, client *http.Client, source sourceURLHLSStream, folder string) (string, error) {
	path, err := prepareSignedHLSPlaylist(ctx, client, source.URL, folder)
	if err == nil || len(source.MPDPlaylist) == 0 || ctx.Err() != nil {
		return path, err
	}
	// A failed nested .m3u8 fetch can leave partial local manifests behind.
	// Clear only this per-download temporary directory before synthesizing one.
	if cleanupErr := os.RemoveAll(folder); cleanupErr != nil {
		return "", fmt.Errorf("MPD 재생목록의 임시 폴더를 정리할 수 없습니다")
	}
	if mkdirErr := os.MkdirAll(folder, 0700); mkdirErr != nil {
		return "", fmt.Errorf("MPD 재생목록 임시 폴더를 만들 수 없습니다")
	}
	path = filepath.Join(folder, "playlist-01.m3u8")
	if saveErr := os.WriteFile(path, source.MPDPlaylist, 0600); saveErr != nil {
		return "", fmt.Errorf("MPD 재생목록을 저장할 수 없습니다")
	}
	return path, nil
}
