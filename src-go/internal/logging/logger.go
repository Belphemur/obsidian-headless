package logging

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

func NewConsoleLogger(output io.Writer) zerolog.Logger {
	writer := zerolog.ConsoleWriter{Out: output}
	return zerolog.New(writer).With().Timestamp().Logger()
}

func NewFileLogger(stdout io.Writer, path string) (zerolog.Logger, func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return zerolog.Logger{}, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return zerolog.Logger{}, nil, err
	}
	console := zerolog.ConsoleWriter{Out: stdout}
	logger := zerolog.New(io.MultiWriter(console, file)).With().Timestamp().Logger()
	cleanup := func() {
		_ = file.Close()
	}
	return logger, cleanup, nil
}
