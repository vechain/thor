// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package log

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/mattn/go-isatty"
)

const errorKey = "LOG_ERROR"

const (
	LegacyLevelCrit = iota
	LegacyLevelError
	LegacyLevelWarn
	LegacyLevelInfo
	LegacyLevelDebug
	LegacyLevelTrace
)

const (
	levelMaxVerbosity slog.Level = math.MinInt
	LevelTrace        slog.Level = -8
	LevelDebug                   = slog.LevelDebug
	LevelInfo                    = slog.LevelInfo
	LevelWarn                    = slog.LevelWarn
	LevelError                   = slog.LevelError
	LevelCrit         slog.Level = 12

	// for backward-compatibility
	LvlTrace = LevelTrace
	LvlInfo  = LevelInfo
	LvlDebug = LevelDebug
)

// convert from old Geth verbosity level constants
// to levels defined by slog
func FromLegacyLevel(lvl int) slog.Level {
	switch lvl {
	case LegacyLevelCrit:
		return LevelCrit
	case LegacyLevelError:
		return slog.LevelError
	case LegacyLevelWarn:
		return slog.LevelWarn
	case LegacyLevelInfo:
		return slog.LevelInfo
	case LegacyLevelDebug:
		return slog.LevelDebug
	case LegacyLevelTrace:
		return LevelTrace
	default:
		break
	}

	// TODO: should we allow use of custom levels or force them to match existing max/min if they fall outside the range as I am doing here?
	if lvl > LegacyLevelTrace {
		return LevelTrace
	}
	return LevelCrit
}

// LevelAlignedString returns a 5-character string containing the name of a Lvl.
func LevelAlignedString(l slog.Level) string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO "
	case slog.LevelWarn:
		return "WARN "
	case slog.LevelError:
		return "ERROR"
	case LevelCrit:
		return "CRIT "
	default:
		return "unknown level"
	}
}

// LevelString returns a string containing the name of a Lvl.
func LevelString(l slog.Level) string {
	switch l {
	case LevelTrace:
		return "trace"
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	case LevelCrit:
		return "crit"
	default:
		return "unknown"
	}
}

// A Logger writes key/value pairs to a Handler
type Logger interface {
	// With returns a new Logger that has this logger's attributes plus the given attributes
	With(ctx ...any) Logger

	// With returns a new Logger that has this logger's attributes plus the given attributes. Identical to 'With'.
	New(ctx ...any) Logger

	// Log logs a message at the specified level with context key/value pairs
	Log(level slog.Level, msg string, ctx ...any)

	// Trace log a message at the trace level with context key/value pairs
	Trace(msg string, ctx ...any)

	// Debug logs a message at the debug level with context key/value pairs
	Debug(msg string, ctx ...any)

	// Info logs a message at the info level with context key/value pairs
	Info(msg string, ctx ...any)

	// Warn logs a message at the warn level with context key/value pairs
	Warn(msg string, ctx ...any)

	// Error logs a message at the error level with context key/value pairs
	Error(msg string, ctx ...any)

	// Crit logs a message at the crit level with context key/value pairs, and exits
	Crit(msg string, ctx ...any)

	// Write logs a message at the specified level
	Write(level slog.Level, msg string, attrs ...any)

	// Enabled reports whether l emits log records at the given context and level.
	Enabled(ctx context.Context, level slog.Level) bool

	// Handler returns the underlying handler of the inner logger.
	Handler() slog.Handler
}

type logger struct {
	inner *slog.Logger
}

func New(jsonLogs bool, level *slog.LevelVar) Logger {
	output := io.Writer(os.Stdout)
	var handler slog.Handler
	if jsonLogs {
		handler = JSONHandlerWithLevel(output, level)
	} else {
		useColor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
		handler = NewTerminalHandlerWithLevel(output, level, useColor)
	}
	return NewLogger(handler)
}

// NewLogger returns a logger with the specified handler set
func NewLogger(h slog.Handler) Logger {
	return &logger{
		slog.New(h),
	}
}

func (l *logger) Handler() slog.Handler {
	return l.inner.Handler()
}

// Write logs a message at the specified level:
func (l *logger) Write(level slog.Level, msg string, attrs ...any) {
	if !l.inner.Enabled(context.Background(), level) {
		return
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	if len(attrs)%2 != 0 {
		attrs = append(attrs, nil, errorKey, "Normalized odd number of arguments by adding nil")
	}
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(attrs...)
	l.inner.Handler().Handle(context.Background(), r)
}

func (l *logger) Log(level slog.Level, msg string, attrs ...any) {
	l.Write(level, msg, attrs...)
}

func (l *logger) With(ctx ...any) Logger {
	return &logger{l.inner.With(ctx...)}
}

func (l *logger) New(ctx ...any) Logger {
	return l.With(ctx...)
}

// Enabled reports whether l emits log records at the given context and level.
func (l *logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l.inner.Enabled(ctx, level)
}

func (l *logger) Trace(msg string, ctx ...any) {
	l.Write(LevelTrace, msg, ctx...)
}

func (l *logger) Debug(msg string, ctx ...any) {
	l.Write(slog.LevelDebug, msg, ctx...)
}

func (l *logger) Info(msg string, ctx ...any) {
	l.Write(slog.LevelInfo, msg, ctx...)
}

func (l *logger) Warn(msg string, ctx ...any) {
	l.Write(slog.LevelWarn, msg, ctx...)
}

func (l *logger) Error(msg string, ctx ...any) {
	l.Write(slog.LevelError, msg, ctx...)
}

func (l *logger) Crit(msg string, ctx ...any) {
	l.Write(LevelCrit, msg, ctx...)
	os.Exit(1)
}
