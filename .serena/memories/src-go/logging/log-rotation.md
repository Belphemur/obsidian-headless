## Log Rotation for sync.log

The `sync.log` for each vault uses **lumberjack** for automatic rotation:

- **MaxSize**: 10 MB — rotates when the file exceeds this size
- **MaxAge**: 3 days — deletes rotated files older than 3 days
- **MaxBackups**: 0 (unlimited) — age limit governs cleanup
- **LocalTime**: true — timestamps in rotated filenames use local time
- **Compress**: false

### Implementation

Located in `src-go/internal/logging/logger.go`:

- `NewFileLogger` creates a `zerolog.Logger` that writes to both console (`stderr`) and a `lumberjack.Logger`.
- The returned cleanup function closes the lumberjack writer.

### Dependencies

- `github.com/rs/zerolog` — structured logging
- `gopkg.in/natefinch/lumberjack.v2` — log rotation
