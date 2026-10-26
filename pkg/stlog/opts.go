package stlog

import (
	"io"
	"log/slog"
)

type Options struct {
	*slog.HandlerOptions
	Writer  io.Writer
	JSON    bool
	Default bool
}

type Option func(o *Options)

func WithSource(v ...bool) Option {
	return func(o *Options) {
		o.AddSource = true
		if len(v) > 0 {
			o.AddSource = v[0]
		}
	}
}

func WithSlogLevel(level slog.Leveler) Option {
	return func(o *Options) {
		o.Level = level
	}
}

func WithLevel(level string) Option {
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(level)); err != nil {
		return func(o *Options) {}
	}
	return WithSlogLevel(slogLevel)
}

func WithWriter(writers ...io.Writer) Option {
	return func(o *Options) {
		if len(writers) == 0 {
			return
		}
		o.Writer = io.MultiWriter(writers...)
	}
}

func WithJSON(v ...bool) Option {
	return func(o *Options) {
		o.JSON = true
		if len(v) > 0 {
			o.JSON = v[0]
		}
	}
}

func WithDefault(v ...bool) Option {
	return func(o *Options) {
		o.Default = true
		if len(v) > 0 {
			o.Default = v[0]
		}
	}
}
