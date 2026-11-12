package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Config controls logger initialization. Zero values mean info level, console
// output on stdout and RFC3339Nano timestamps.
type Config struct {
	Level      string
	Format     string // "json", anything else is console text
	TimeFormat string
	Fields     map[string]any
	Out        io.Writer
}

// Logger wraps zerolog with a slog.Attr API.
type Logger struct {
	z zerolog.Logger
}

// New creates a logger.
func New(cfg Config) *Logger {
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = time.RFC3339Nano
	}
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}
	zerolog.TimeFieldFormat = cfg.TimeFormat
	zerolog.SetGlobalLevel(parseLevel(cfg.Level))

	writer := cfg.Out
	if !strings.EqualFold(cfg.Format, "json") {
		writer = zerolog.ConsoleWriter{Out: cfg.Out, TimeFormat: cfg.TimeFormat}
	}

	ctx := zerolog.New(writer).With().Timestamp().Int("pid", os.Getpid())
	for k, v := range cfg.Fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{z: ctx.Logger()}
}

func parseLevel(level string) zerolog.Level {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "warning" {
		return zerolog.WarnLevel
	}
	parsed, err := zerolog.ParseLevel(level)
	if err != nil || parsed == zerolog.NoLevel {
		return zerolog.InfoLevel
	}
	return parsed
}

// Info logs at info level.
func (l *Logger) Info(msg string, attrs ...slog.Attr) { l.write(zerolog.InfoLevel, msg, attrs) }

// Warn logs at warn level.
func (l *Logger) Warn(msg string, attrs ...slog.Attr) { l.write(zerolog.WarnLevel, msg, attrs) }

// Error logs at error level.
func (l *Logger) Error(msg string, attrs ...slog.Attr) { l.write(zerolog.ErrorLevel, msg, attrs) }

func (l *Logger) write(level zerolog.Level, msg string, attrs []slog.Attr) {
	if l == nil {
		return
	}
	ev := l.z.WithLevel(level)
	for _, attr := range attrs {
		switch attr.Value.Kind() { // typed writers allocate nothing, Interface does
		case slog.KindString:
			ev.Str(attr.Key, attr.Value.String())
		case slog.KindInt64:
			ev.Int64(attr.Key, attr.Value.Int64())
		case slog.KindBool:
			ev.Bool(attr.Key, attr.Value.Bool())
		case slog.KindDuration:
			ev.Str(attr.Key, attr.Value.Duration().String()) // "12.3ms": the unit travels with the value
		default:
			ev.Interface(attr.Key, attr.Value.Any())
		}
	}
	ev.Msg(msg)
}

// Zerolog returns the underlying zerolog logger.
func (l *Logger) Zerolog() *zerolog.Logger {
	if l == nil {
		return nil
	}
	return &l.z
}
