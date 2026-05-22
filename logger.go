package pcanbasic

// Logger 是本库内部使用的极简日志接口。
//
// 默认使用 noopLogger（什么都不打印）。如需接入 slog/zap/logrus，
// 实现下面三个方法并通过 WithLogger 注入即可。
//
// 接口故意保持最小：库本身只在少数地方打 Debug/Info/Warn 日志，
// 不输出 Error 级（错误通过 *Error/Errors() 返回给调用方）。
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// noopLogger 是默认实现：所有方法都是 no-op。
type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
