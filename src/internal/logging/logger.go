package logging

import (
	"io"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

func NewConsoleLogger(output io.Writer) zerolog.Logger {
	writer := zerolog.ConsoleWriter{Out: output, TimeFormat: "15:04:05.000"}
	return zerolog.New(writer).With().Timestamp().Logger()
}

func NewFileLogger(stdout io.Writer, path string) (zerolog.Logger, func(), error) {
	file := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    10, // megabytes
		MaxAge:     3,  // days
		MaxBackups: 0,  // unlimited, age limit governs cleanup
		LocalTime:  true,
		Compress:   true,
	}
	console := zerolog.ConsoleWriter{Out: stdout, TimeFormat: "15:04:05.000"}
	logger := zerolog.New(io.MultiWriter(console, file)).With().Timestamp().Logger()
	cleanup := func() {
		_ = file.Close()
	}
	return logger, cleanup, nil
}
