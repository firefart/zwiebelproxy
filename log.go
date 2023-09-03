package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"
)

type log struct {
	logLevel *slog.LevelVar
	logger   *slog.Logger
}

func NewLogger(w io.Writer) *log {
	logLevel := &slog.LevelVar{}
	logLevel.Set(slog.LevelInfo)
	replace := func(groups []string, a slog.Attr) slog.Attr {
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}
	return &log{
		logLevel: logLevel,
		logger: slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
			Level:       logLevel,
			AddSource:   true,
			ReplaceAttr: replace,
		})),
	}
}

func (l *log) SetLevel(s slog.Level) {
	l.logLevel.Set(s)
}

func (l *log) Debug(arg string) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelDebug, arg, pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Debugf(format string, args ...interface{}) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelDebug, fmt.Sprintf(format, args...), pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Infof(format string, args ...interface{}) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Info(arg string) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelInfo, arg, pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Error(arg string) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelError, arg, pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Errorf(format string, args ...interface{}) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Warn(arg string) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelWarn, arg, pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}

func (l *log) Warnf(format string, args ...interface{}) {
	if !l.logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprintf(format, args...), pcs[0])
	_ = l.logger.Handler().Handle(context.Background(), r)
}
