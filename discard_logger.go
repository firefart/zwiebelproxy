package main

type DiscardLogger struct{}

func (l *DiscardLogger) Debugf(format string, args ...interface{}) {}
func (l *DiscardLogger) Infof(format string, args ...interface{})  {}
func (l *DiscardLogger) Warnf(format string, args ...interface{})  {}
func (l *DiscardLogger) Errorf(format string, args ...interface{}) {}
func (l *DiscardLogger) Fatalf(format string, args ...interface{}) {}
func (l *DiscardLogger) Debug(args ...interface{})                 {}
func (l *DiscardLogger) Info(args ...interface{})                  {}
func (l *DiscardLogger) Warn(args ...interface{})                  {}
func (l *DiscardLogger) Error(args ...interface{})                 {}
func (l *DiscardLogger) Fatal(args ...interface{})                 {}
