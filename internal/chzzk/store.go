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

func normalizeStoredChannel(channel Channel) (Channel, error) {
	channel.ChannelID = strings.ToLower(strings.TrimSpace(channel.ChannelID))
	channel.ChannelName = strings.TrimSpace(channel.ChannelName)
	if err := channel.validate(); err != nil {
		return Channel{}, err
	}
	channel.ChannelURL = ChannelURL(channel.ChannelID)
	return channel, nil
}

func (s *Store) Save(channel Channel) ([]Channel, error) {
	channel, err := normalizeStoredChannel(channel)
	if err != nil {
		return nil, err
	}

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

func (s *Store) ReplaceAll(channels []Channel) error {
	next := make(map[string]Channel, len(channels))
	for _, channel := range channels {
		normalized, err := normalizeStoredChannel(channel)
		if err != nil {
			return err
		}
		if _, exists := next[normalized.ChannelID]; exists {
			return fmt.Errorf("중복된 저장 채널입니다: %s", normalized.ChannelID)
		}
		next[normalized.ChannelID] = normalized
	}

	s.mu.Lock()
	s.channels = next
	s.mu.Unlock()
	return nil
}
