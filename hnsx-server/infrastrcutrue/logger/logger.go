package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var defaultLogger *zap.Logger

func Init(env string) error {
	config := zap.NewProductionConfig()

	if env == "development" {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	} else {
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.CallerKey = "caller"
		config.EncoderConfig.StacktraceKey = "stacktrace"
	}

	// 动态日志级别
	level := zap.NewAtomicLevel()
	if env == "development" {
		level.SetLevel(zapcore.DebugLevel)
	} else {
		level.SetLevel(zapcore.InfoLevel)
	}
	config.Level = level

	// 多输出：stdout + 文件
	fileEncoder := zapcore.NewJSONEncoder(config.EncoderConfig)
	consoleEncoder := zapcore.NewConsoleEncoder(config.EncoderConfig)

	file, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level), // 控制台输出
		zapcore.NewCore(fileEncoder, zapcore.AddSync(file), level),         // 文件输出
	)

	// 添加调用者信息和堆栈跟踪
	defaultLogger = zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(zap.String("service", "myapp")),
	)

	return nil
}

func Logger() *zap.Logger {
	if defaultLogger == nil {
		defaultLogger, _ = zap.NewProduction()
	}
	return defaultLogger
}

// Sugar 返回 SugaredLogger，支持 fmt.Sprintf 风格
func Sugar() *zap.SugaredLogger {
	return Logger().Sugar()
}

// 包装方法，方便全局调用
func Info(msg string, fields ...zap.Field)  { Logger().Info(msg, fields...) }
func Debug(msg string, fields ...zap.Field) { Logger().Debug(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { Logger().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { Logger().Error(msg, fields...) }
func Fatal(msg string, fields ...zap.Field) { Logger().Fatal(msg, fields...) }
