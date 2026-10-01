package chzzk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testChannelID = "6e06f5e1907f17eff543abd06cb62891"

func TestParseChannelURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{name: "https", in: "https://chzzk.naver.com/" + testChannelID, ok: true},
		{name: "without scheme", in: "chzzk.naver.com/" + testChannelID + "/", ok: true},
		{name: "query", in: "https://chzzk.naver.com/" + testChannelID + "?foo=bar", ok: true},
		{name: "wrong host", in: "https://example.com/" + testChannelID, ok: false},
		{name: "wrong path", in: "https://chzzk.naver.com/live/" + testChannelID, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, normalized, err := ParseChannelURL(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("expected error, got id=%s", id)
			}
			if tt.ok && (id != testChannelID || normalized != ChannelURL(testChannelID)) {
				t.Fatalf("unexpected parse result: %s %s", id, normalized)
			}
		})
	}
}

func TestSearchChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/service/v1/search/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("keyword") != "테스트" || r.URL.Query().Get("offset") != "0" || r.URL.Query().Get("size") != "20" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"size":1,"page":{"next":{"offset":20}},"data":[{"channel":{"channelId":"6e06f5e1907f17eff543abd06cb62891","channelName":"테스트 채널","channelImageUrl":"https://example.com/a.png","verifiedMark":true,"channelDescription":"설명","followerCount":1234,"openLive":false}}]}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	result, err := client.SearchChannels(context.Background(), "테스트", 0, 20)
	if err != nil {
		t.Fatalf("SearchChannels returned error: %v", err)
	}
	if len(result.Channels) != 1 || result.Channels[0].ChannelName != "테스트 채널" {
		t.Fatalf("unexpected channels: %#v", result.Channels)
	}
	if !result.HasNext || result.NextOffset != 20 {
		t.Fatalf("unexpected pagination: %#v", result)
	}
}

func TestGetChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":null,"content":{"channelId":"6e06f5e1907f17eff543abd06cb62891","channelName":"테스트 채널","channelImageUrl":"","verifiedMark":false,"channelDescription":"","followerCount":10,"openLive":true}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	channel, err := client.GetChannel(context.Background(), testChannelID)
	if err != nil {
		t.Fatalf("GetChannel returned error: %v", err)
	}
	if channel.ChannelURL != ChannelURL(testChannelID) || !channel.OpenLive {
		t.Fatalf("unexpected channel: %#v", channel)
	}
}

func TestStoreDeduplicatesChannelID(t *testing.T) {
	store := NewStore()
	channel := Channel{ChannelID: testChannelID, ChannelName: "테스트"}
	if _, err := store.Save(channel); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if _, err := store.Save(channel); err == nil {
		t.Fatal("expected duplicate save error")
	}
	if got := len(store.List()); got != 1 {
		t.Fatalf("expected 1 channel, got %d", got)
	}
}


func TestStoreReplaceAllRestoresChannels(t *testing.T) {
	store := NewStore()
	secondID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	err := store.ReplaceAll([]Channel{
		{ChannelID: testChannelID, ChannelName: "Z 채널"},
		{ChannelID: secondID, ChannelName: "A 채널"},
	})
	if err != nil {
		t.Fatal(err)
	}

	channels := store.List()
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].ChannelID != secondID || channels[0].ChannelURL != ChannelURL(secondID) {
		t.Fatalf("unexpected restored channels: %#v", channels)
	}
}

func TestStoreReplaceAllRejectsInvalidChannelWithoutChangingStore(t *testing.T) {
	store := NewStore()
	if _, err := store.Save(Channel{ChannelID: testChannelID, ChannelName: "기존"}); err != nil {
		t.Fatal(err)
	}

	if err := store.ReplaceAll([]Channel{{ChannelID: "invalid", ChannelName: "잘못됨"}}); err == nil {
		t.Fatal("expected invalid restore error")
	}

	channels := store.List()
	if len(channels) != 1 || channels[0].ChannelID != testChannelID {
		t.Fatalf("store changed after failed restore: %#v", channels)
	}
}
