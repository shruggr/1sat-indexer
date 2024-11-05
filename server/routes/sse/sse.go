package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Session struct {
	StateChannel chan *redis.Message
	Topics       []string
}

type SessionsLock struct {
	MU         sync.Mutex
	Sessions   []*Session
	Topics     map[string][]*Session
	AddSubs    chan []string
	RemoveSubs chan []string
}

func (sl *SessionsLock) AddSession(s *Session) {
	sl.MU.Lock()
	sl.Sessions = append(sl.Sessions, s)
	var newSubs []string
	for _, topic := range s.Topics {
		if sessions, ok := sl.Topics[topic]; ok {
			sessions = append(sessions, s)
		} else {
			sl.Topics[topic] = []*Session{s}
			newSubs = append(newSubs, topic)
		}
	}
	sl.MU.Unlock()
	if len(newSubs) > 0 {
		sl.AddSubs <- newSubs
	}
}

func (sl *SessionsLock) RemoveSession(s *Session) {
	sl.MU.Lock()
	var removeSubs []string
	idx := slices.Index(sl.Sessions, s)
	if idx != -1 {
		for _, topic := range s.Topics {
			if sessions, ok := sl.Topics[topic]; ok {
				idx := slices.Index(sessions, s)
				if idx != -1 {
					sessions = slices.Delete(sessions, idx, idx+1)
				}
				if len(sessions) == 0 {
					delete(sl.Topics, topic)
					removeSubs = append(removeSubs, topic)
				}
			}
		}
		sl.Sessions[idx] = nil
		sl.Sessions = slices.Delete(sl.Sessions, idx, idx+1)
	}
	sl.MU.Unlock()
	if len(removeSubs) > 0 {
		sl.RemoveSubs <- removeSubs
	}
}

func FormatSSEMessage(eventType string, data any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	m := map[string]any{
		"data": data,
	}

	err := enc.Encode(m)
	if err != nil {
		return "", nil
	}
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("event: %s\n", eventType))
	if data != nil {
		sb.WriteString(fmt.Sprintf("data: %v\n\n", buf.String()))
	}
	return sb.String(), nil
}
