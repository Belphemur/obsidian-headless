package watch

import (
	"os"
	"sync"
	"syscall"
	"time"
)

type FileState struct {
	ModTime time.Time
	Size    int64
	Mode    os.FileMode
	Ino     uint64
}

type Scanner struct {
	mu    sync.RWMutex
	state map[string]FileState
}

func getInode(info os.FileInfo) uint64 {
	if info == nil {
		return 0
	}
	// Use syscall.Stat_t for Unix-like platforms; returns 0 on Windows.
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return stat.Ino
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
	current := FileState{ModTime: info.ModTime(), Size: info.Size(), Mode: info.Mode(), Ino: getInode(info)}
	if !known || previous.ModTime != current.ModTime || previous.Size != current.Size || previous.Mode != current.Mode || previous.Ino != current.Ino {
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
	s.UpdateInfo(path, info)
}

func (s *Scanner) UpdateInfo(path string, info os.FileInfo) {
	s.mu.Lock()
	s.state[path] = FileState{ModTime: info.ModTime(), Size: info.Size(), Mode: info.Mode(), Ino: getInode(info)}
	s.mu.Unlock()
}

func (s *Scanner) GetInode(path string) (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fs, ok := s.state[path]
	if !ok {
		return 0, false
	}
	return fs.Ino, true
}

func (s *Scanner) Remove(path string) {
	s.mu.Lock()
	delete(s.state, path)
	s.mu.Unlock()
}
