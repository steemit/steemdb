package services

import (
	"go.uber.org/zap"

	"github.com/steemit/steemdb/sync/internal/utils"
)

// TestLogger implements utils.Logger for testing
type TestLogger struct{}

func (t *TestLogger) Debug(msg string, fields ...zap.Field) {}
func (t *TestLogger) Info(msg string, fields ...zap.Field)  {}
func (t *TestLogger) Warn(msg string, fields ...zap.Field)  {}
func (t *TestLogger) Error(msg string, fields ...zap.Field) {}
func (t *TestLogger) Fatal(msg string, fields ...zap.Field) {}
func (t *TestLogger) With(fields ...zap.Field) utils.Logger { return t }
func (t *TestLogger) Sync() error                           { return nil }
