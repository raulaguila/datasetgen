package output

import "log/slog"

// Logger records application events without binding the core to a logging library.
type Logger interface {
	Info(string, ...slog.Attr)
	Warn(string, ...slog.Attr)
}

// Log returns a no-op logger for optional application logging.
func Log(log Logger) Logger {
	if log == nil {
		return discardLog{}
	}
	return log
}

type discardLog struct{}

func (discardLog) Info(string, ...slog.Attr) {}
func (discardLog) Warn(string, ...slog.Attr) {}
