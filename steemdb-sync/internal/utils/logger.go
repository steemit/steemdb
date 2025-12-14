package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
	Sync() error
}

type zapLogger struct {
	logger *zap.Logger
}

func (l *zapLogger) Debug(msg string, fields ...zap.Field) {
	l.logger.Debug(msg, fields...)
}

func (l *zapLogger) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

func (l *zapLogger) Warn(msg string, fields ...zap.Field) {
	l.logger.Warn(msg, fields...)
}

func (l *zapLogger) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

func (l *zapLogger) Fatal(msg string, fields ...zap.Field) {
	l.logger.Fatal(msg, fields...)
}

func (l *zapLogger) With(fields ...zap.Field) Logger {
	return &zapLogger{logger: l.logger.With(fields...)}
}

func (l *zapLogger) Sync() error {
	return l.logger.Sync()
}

// NewLogger creates a new logger instance
func NewLogger(config LogConfig) (Logger, error) {
	// Parse log level
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder
	var encoder zapcore.Encoder
	if config.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Create writer syncer
	var writeSyncer zapcore.WriteSyncer
	if config.File != "" {
		// Resolve log file path (handle relative paths)
		logFile := config.File
		if !filepath.IsAbs(logFile) {
			// If relative path, resolve relative to current working directory
			// Users can also use absolute paths like /var/log/steemdb-sync.log
			cwd, err := os.Getwd()
			if err == nil {
				logFile = filepath.Join(cwd, logFile)
			}
		}
		
		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			// If cannot create directory, fall back to console only
			fmt.Fprintf(os.Stderr, "Warning: failed to create log directory %s: %v, using console output only\n", logDir, err)
			writeSyncer = zapcore.AddSync(os.Stdout)
		} else {
			// Try to open log file to check write permissions
			testFile, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				// If cannot write to log file, fall back to console only
				fmt.Fprintf(os.Stderr, "Warning: cannot write to log file %s: %v, using console output only\n", logFile, err)
				writeSyncer = zapcore.AddSync(os.Stdout)
			} else {
				testFile.Close()
				// File output with rotation
				fileWriter := &lumberjack.Logger{
					Filename:   logFile,
					MaxSize:    config.MaxSize,
					MaxBackups: config.MaxBackups,
					MaxAge:     config.MaxAge,
					Compress:   true,
				}
				writeSyncer = zapcore.NewMultiWriteSyncer(
					zapcore.AddSync(fileWriter),
					zapcore.AddSync(os.Stdout),
				)
			}
		}
	} else {
		// Console output only
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// Create core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// Create logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &zapLogger{logger: logger}, nil
}

// Field helpers
func String(key, val string) zap.Field {
	return zap.String(key, val)
}

func Int(key string, val int) zap.Field {
	return zap.Int(key, val)
}

func Int64(key string, val int64) zap.Field {
	return zap.Int64(key, val)
}

func Float64(key string, val float64) zap.Field {
	return zap.Float64(key, val)
}

func Bool(key string, val bool) zap.Field {
	return zap.Bool(key, val)
}

func Error(err error) zap.Field {
	return zap.Error(err)
}

func Duration(key string, val interface{}) zap.Field {
	return zap.Duration(key, val.(time.Duration))
}

func Any(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}
