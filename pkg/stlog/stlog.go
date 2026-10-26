package stlog

import (
	"log/slog"
	"os"
)

func New(opts ...Option) *slog.Logger {
	options := &Options{
		HandlerOptions: &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelInfo,
		},
		Writer:  os.Stdout,
		JSON:    true,
		Default: true,
	}
	for _, opt := range opts {
		opt(options)
	}

	var handler slog.Handler
	if options.JSON {
		handler = slog.NewJSONHandler(options.Writer, options.HandlerOptions)
	} else {
		handler = slog.NewTextHandler(options.Writer, options.HandlerOptions)
	}

	logger := slog.New(handler)
	if options.Default {
		slog.SetDefault(logger)
	}
	return logger
}
