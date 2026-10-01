package chzzk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetChannelVideos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/service/v1/channels/"+testChannelID+"/videos" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("sortType") != "LATEST" || query.Get("pagingType") != "PAGE" || query.Get("page") != "1" || query.Get("size") != "24" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if _, ok := query["publishDateAt"]; !ok {
			t.Fatal("publishDateAt query is missing")
		}
		if _, ok := query["videoType"]; !ok {
			t.Fatal("videoType query is missing")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"page":1,"size":24,"totalCount":50,"totalPages":3,"data":[{"videoNo":12888749,"videoId":"C048BAFAB2B98EE8A9A43EB3444438AC6FC3","videoTitle":"테스트 다시보기","videoType":"REPLAY","publishDate":"2026-09-30 20:10:00","publishDateAt":1790770200000,"thumbnailImageUrl":"https://example.com/thumb.jpg","trailerUrl":null,"duration":3661,"readCount":12345,"livePv":12000,"categoryType":"GAME","videoCategory":"TestGame","videoCategoryValue":"테스트 게임","exposure":true,"adult":false,"clipActive":true,"commentActive":true,"chapterActive":false,"tags":["태그1"],"channel":{"channelId":"6e06f5e1907f17eff543abd06cb62891","channelName":"테스트 채널","channelImageUrl":"https://example.com/channel.png","verifiedMark":true}}]}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	result, err := client.GetChannelVideos(context.Background(), testChannelID, 1, 24)
	if err != nil {
		t.Fatalf("GetChannelVideos returned error: %v", err)
	}
	if len(result.Videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(result.Videos))
	}

	video := result.Videos[0]
	if video.VideoNo != 12888749 || video.VideoTitle != "테스트 다시보기" || video.VideoURL != "https://chzzk.naver.com/video/12888749" {
		t.Fatalf("unexpected video: %#v", video)
	}
	if video.Channel.ChannelID != testChannelID || !video.Channel.VerifiedMark {
		t.Fatalf("unexpected video channel: %#v", video.Channel)
	}
	if !video.Exposure || !video.ClipActive || !video.CommentActive {
		t.Fatalf("unexpected video flags: %#v", video)
	}
	if !result.HasNext || result.NextPage != 2 || result.TotalCount != 50 || result.TotalPages != 3 {
		t.Fatalf("unexpected pagination: %#v", result)
	}
}

func TestGetChannelVideosClampsPageAndSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("page") != "0" || query.Get("size") != "50" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"page":0,"size":50,"totalCount":0,"totalPages":0,"data":[]}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	result, err := client.GetChannelVideos(context.Background(), testChannelID, -10, 1000)
	if err != nil {
		t.Fatalf("GetChannelVideos returned error: %v", err)
	}
	if result.HasNext || result.NextPage != 0 {
		t.Fatalf("unexpected empty pagination: %#v", result)
	}
}

func TestGetChannelVideosDerivesTotalPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"page":0,"size":24,"totalCount":49,"data":[]}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	result, err := client.GetChannelVideos(context.Background(), testChannelID, 0, 24)
	if err != nil {
		t.Fatalf("GetChannelVideos returned error: %v", err)
	}
	if result.TotalPages != 3 || !result.HasNext || result.NextPage != 1 {
		t.Fatalf("unexpected derived pagination: %#v", result)
	}
}

func TestGetChannelVideosRejectsInvalidChannelID(t *testing.T) {
	client := newClient("https://example.invalid", http.DefaultClient)
	if _, err := client.GetChannelVideos(context.Background(), "not-a-channel", 0, 24); err == nil {
		t.Fatal("expected invalid channel id error")
	}
}

func TestGetChannelVideosHandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":404,"message":"채널 또는 영상을 찾을 수 없습니다.","content":null}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	_, err := client.GetChannelVideos(context.Background(), testChannelID, 0, 24)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected API code error, got %v", err)
	}
}

func TestGetChannelVideosHandlesMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"content":`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	_, err := client.GetChannelVideos(context.Background(), testChannelID, 0, 24)
	if err == nil || !strings.Contains(err.Error(), "응답 형식") {
		t.Fatalf("expected response format error, got %v", err)
	}
}

func TestGetChannelVideosHandlesUnexpectedVideoShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"page":0,"size":24,"totalCount":1,"totalPages":1,"data":[{"videoNo":0,"videoTitle":""}]}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	_, err := client.GetChannelVideos(context.Background(), testChannelID, 0, 24)
	if err == nil || !strings.Contains(err.Error(), "응답 형식") {
		t.Fatalf("expected response shape error, got %v", err)
	}
}
