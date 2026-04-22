package watch

import (
	"os"
	"sync"
	"time"
)

type FileState struct {
	ModTime time.Time
	Size    int64
	Mode    os.FileMode
}

type Scanner struct {
	mu    sync.RWMutex
	state map[string]FileState
}

func NewScanner() *Scanner {
	return &Scanner{state: map[string]FileState{}}
}

func (s *Scanner) HasChanged(path string) (bool, EventType) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Remove(path)
			return true, EventRemove
		}
		return false, EventWrite
	}
	s.mu.RLock()
	previous, known := s.state[path]
	s.mu.RUnlock()
	current := FileState{ModTime: info.ModTime(), Size: info.Size(), Mode: info.Mode()}
	if !known || previous.ModTime != current.ModTime || previous.Size != current.Size || previous.Mode != current.Mode {
		s.mu.Lock()
		s.state[path] = current
		s.mu.Unlock()
		if !known {
			return true, EventCreate
		}
		return true, EventWrite
	}
	return false, EventWrite
}

func (s *Scanner) Update(path string) {
	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.state[path] = FileState{ModTime: info.ModTime(), Size: info.Size(), Mode: info.Mode()}
	s.mu.Unlock()
}

func (s *Scanner) Remove(path string) {
	s.mu.Lock()
	delete(s.state, path)
	s.mu.Unlock()
}
