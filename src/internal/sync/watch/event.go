package watch

import "time"

type EventType int

const (
	EventCreate EventType = iota
	EventWrite
	EventRemove
	EventRename
	EventChmod
)

func (e EventType) String() string {
	switch e {
	case EventCreate:
		return "CREATE"
	case EventWrite:
		return "WRITE"
	case EventRemove:
		return "REMOVE"
	case EventRename:
		return "RENAME"
	case EventChmod:
		return "CHMOD"
	default:
		return "UNKNOWN"
	}
}

type ScanEvent struct {
	Path       string
	Type       EventType
	DetectedAt time.Time
}
