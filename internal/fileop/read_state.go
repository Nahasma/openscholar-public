package fileop

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

type FileReadRecord struct {
	CanonicalPath string
	MTime         time.Time
	Size          int64
	Offset        int
	Limit         int
	ContentHash   string
	Encoding      string
	LineEnding    string
	EditEligible  bool
}

type ReadState struct {
	mu      sync.RWMutex
	records map[string]FileReadRecord
}

func NewReadState() *ReadState {
	return &ReadState{records: make(map[string]FileReadRecord)}
}

func ReadKey(path string, offset, limit int) string {
	return path + "#" + itoa(offset) + ":" + itoa(limit)
}

func (s *ReadState) Put(r FileReadRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[ReadKey(r.CanonicalPath, r.Offset, r.Limit)] = r
}

func (s *ReadState) Get(path string, offset, limit int) (FileReadRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[ReadKey(path, offset, limit)]
	return r, ok
}

func HashContent(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// SessionReadStateManager stores isolated read state per session.
type SessionReadStateManager struct {
	mu     sync.Mutex
	states map[string]*ReadState
}

func NewSessionReadStateManager() *SessionReadStateManager {
	return &SessionReadStateManager{states: make(map[string]*ReadState)}
}

func (m *SessionReadStateManager) Get(sessionID string) *ReadState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.states[sessionID]; ok {
		return s
	}
	s := NewReadState()
	m.states[sessionID] = s
	return s
}
