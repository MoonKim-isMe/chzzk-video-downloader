package chzzk

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

func NewStore() *Store {
	return &Store{channels: make(map[string]Channel)}
}

func (s *Store) Save(channel Channel) ([]Channel, error) {
	channel.ChannelID = strings.ToLower(strings.TrimSpace(channel.ChannelID))
	if err := channel.validate(); err != nil {
		return nil, err
	}
	channel.ChannelURL = ChannelURL(channel.ChannelID)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.channels[channel.ChannelID]; exists {
		return s.listLocked(), fmt.Errorf("이미 저장된 채널입니다")
	}
	s.channels[channel.ChannelID] = channel
	return s.listLocked(), nil
}

func (s *Store) Remove(channelID string) []Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, strings.ToLower(strings.TrimSpace(channelID)))
	return s.listLocked()
}

func (s *Store) List() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked()
}

func (s *Store) listLocked() []Channel {
	channels := make([]Channel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, channel)
	}
	sort.Slice(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].ChannelName) < strings.ToLower(channels[j].ChannelName)
	})
	return channels
}
