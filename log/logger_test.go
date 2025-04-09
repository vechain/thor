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
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/holiman/uint256"
)

func TestTerminalHandlerWithAttrs(t *testing.T) {
	out := new(bytes.Buffer)
	var level slog.LevelVar
	level.Set(LevelTrace)
	handler := NewTerminalHandlerWithLevel(out, &level, false).WithAttrs([]slog.Attr{slog.String("baz", "bat")})
	logger := NewLogger(handler)
	logger.Trace("a message", "foo", "bar")
	have := out.String()
	// The timestamp is locale-dependent, so we want to trim that off
	// "INFO [01-01|00:00:00.000] a message ..." -> "a message..."
	have = strings.Split(have, "]")[1]
	want := " a message                                baz=bat foo=bar\n"
	if have != want {
		t.Errorf("\nhave: %q\nwant: %q\n", have, want)
	}
}

// Make sure the default json handler outputs debug log lines
func TestJSONHandler(t *testing.T) {
	out := new(bytes.Buffer)
	handler := JSONHandler(out)
	logger := slog.New(handler)
	logger.Debug("hi there")
	if len(out.String()) == 0 {
		t.Error("expected non-empty debug log output from default JSON Handler")
	}

	out.Reset()

	var level slog.LevelVar
	level.Set(LevelInfo)

	handler = JSONHandlerWithLevel(out, &level)
	logger = slog.New(handler)
	logger.Debug("hi there")
	if len(out.String()) != 0 {
		t.Errorf("expected empty debug log output, but got: %v", out.String())
	}
}

func BenchmarkTraceLogging(b *testing.B) {
	SetDefault(NewLogger(NewTerminalHandler(os.Stderr, true)))

	for i := 0; b.Loop(); i++ {
		Trace("a message", "v", i)
	}
}

func BenchmarkTerminalHandler(b *testing.B) {
	l := NewLogger(NewTerminalHandler(io.Discard, false))
	benchmarkLogger(b, l)
}
func BenchmarkLogfmtHandler(b *testing.B) {
	l := NewLogger(LogfmtHandler(io.Discard))
	benchmarkLogger(b, l)
}

func BenchmarkJSONHandler(b *testing.B) {
	l := NewLogger(JSONHandler(io.Discard))
	benchmarkLogger(b, l)
}

func benchmarkLogger(b *testing.B, l Logger) {
	var (
		bb     = make([]byte, 10)
		tt     = time.Now()
		bigint = big.NewInt(100)
		nilbig *big.Int
		err    = errors.New("oh nooes it's crap")
	)
	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		l.Info("This is a message",
			"foo", int16(i),
			"bytes", bb,
			"bonk", "a string with text",
			"time", tt,
			"bigint", bigint,
			"nilbig", nilbig,
			"err", err)
	}
	b.StopTimer()
}

func TestLoggerOutput(t *testing.T) {
	type custom struct {
		A string
		B int8
	}
	var (
		customA   = custom{"Foo", 12}
		customB   = custom{"Foo\nLinebreak", 122}
		bb        = make([]byte, 10)
		tt        = time.Time{}
		bigint    = big.NewInt(100)
		nilbig    *big.Int
		err       = errors.New("oh nooes it's crap")
		smallUint = uint256.NewInt(500_000)
		bigUint   = &uint256.Int{0xff, 0xff, 0xff, 0xff}
	)

	out := new(bytes.Buffer)
	var level slog.LevelVar
	level.Set(LevelInfo)
	handler := NewTerminalHandlerWithLevel(out, &level, false)
	NewLogger(handler).Info("This is a message",
		"foo", int16(123),
		"bytes", bb,
		"bonk", "a string with text",
		"time", tt,
		"bigint", bigint,
		"nilbig", nilbig,
		"err", err,
		"struct", customA,
		"struct", customB,
		"ptrstruct", &customA,
		"smalluint", smallUint,
		"bigUint", bigUint)

	have := out.String()
	t.Logf("output %v", out.String())
	want := `INFO [11-07|19:14:33.821] This is a message                        foo=123 bytes="[0 0 0 0 0 0 0 0 0 0]" bonk="a string with text" time=0001-01-01T00:00:00+0000 bigint=100 nilbig=<nil> err="oh nooes it's crap" struct="{A:Foo B:12}" struct="{A:Foo\nLinebreak B:122}" ptrstruct="&{A:Foo B:12}" smalluint=500,000 bigUint=1,600,660,942,523,603,594,864,898,306,482,794,244,293,965,082,972,225,630,372,095
`
	if !bytes.Equal([]byte(have)[25:], []byte(want)[25:]) {
		t.Errorf("Error\nhave: %q\nwant: %q", have, want)
	}
}

const termTimeFormat = "01-02|15:04:05.000"

func BenchmarkAppendFormat(b *testing.B) {
	var now = time.Now()
	b.Run("fmt time.Format", func(b *testing.B) {
		for b.Loop() {
			fmt.Fprintf(io.Discard, "%s", now.Format(termTimeFormat))
		}
	})
	b.Run("time.AppendFormat", func(b *testing.B) {
		for b.Loop() {
			now.AppendFormat(nil, termTimeFormat)
		}
	})
	var buf = new(bytes.Buffer)
	b.Run("time.Custom", func(b *testing.B) {
		for b.Loop() {
			writeTimeTermFormat(buf, now)
			buf.Reset()
		}
	})
}

func TestTermTimeFormat(t *testing.T) {
	var now = time.Now()
	want := now.AppendFormat(nil, termTimeFormat)
	var b = new(bytes.Buffer)
	writeTimeTermFormat(b, now)
	have := b.Bytes()
	if !bytes.Equal(have, want) {
		t.Errorf("have != want\nhave: %q\nwant: %q\n", have, want)
	}
}
