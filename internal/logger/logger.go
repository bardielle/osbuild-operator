package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Logger() (error, *zap.SugaredLogger) {
	var level zapcore.Level
	err := level.UnmarshalText([]byte("info"))
	if err != nil {
		return err, nil
	}
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level.SetLevel(level)
	logBuilder, err := logConfig.Build()
	if err != nil {
		return err, nil
	}
	return nil, logBuilder.Sugar()
}
