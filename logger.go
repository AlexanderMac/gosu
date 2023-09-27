package gosu

import "log"

type Logger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)

	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type defLogger struct{}

var logger Logger = &defLogger{}

func SetLogger(l Logger) {
	logger = l
}

func (l *defLogger) Debug(args ...any) {
	log.Println(args...)
}

func (l *defLogger) Info(args ...any) {
	log.Println(args...)
}

func (l *defLogger) Warn(args ...any) {
	log.Println(args...)
}

func (l *defLogger) Error(args ...any) {
	log.Println(args...)
}

func (l *defLogger) Debugf(format string, args ...any) {
	log.Printf(format, args...)
}

func (l *defLogger) Infof(format string, args ...any) {
	log.Printf(format, args...)
}

func (l *defLogger) Warnf(format string, args ...any) {
	log.Printf(format, args...)
}

func (l *defLogger) Errorf(format string, args ...any) {
	log.Printf(format, args...)
}
