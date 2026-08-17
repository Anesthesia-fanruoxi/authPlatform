// Package common 统一日志：全部输出走本文件定义，格式为
// 时间 [级别] 文件:行号 [接口名] 日志内容。
package common

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"
)

// 日志级别。
const (
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

func init() {
	// 去掉标准 log 默认前缀，时间/位置由 writeLog 统一组装
	log.SetFlags(0)
}

// LogInfo 输出 INFO 日志。apiName 为接口名（HTTP 接口传方法+路径，如 "POST /api/auth/verify"；
// 后台任务传模块名，如 "cleanup"）。
func LogInfo(apiName, format string, args ...any) {
	writeLog(LevelInfo, apiName, format, args...)
}

// LogWarn 输出 WARN 日志。
func LogWarn(apiName, format string, args ...any) {
	writeLog(LevelWarn, apiName, format, args...)
}

// LogError 输出 ERROR 日志。
func LogError(apiName, format string, args ...any) {
	writeLog(LevelError, apiName, format, args...)
}

// writeLog 组装统一格式：时间 [级别] 文件:行号 [接口名] 内容。
// 调用点取栈上第一个非本文件的帧，保证文件/行号指向真实业务代码。
func writeLog(level, apiName, format string, args ...any) {
	file, line := caller(3)
	msg := fmt.Sprintf(format, args...)
	log.Printf("%s [%s] %s:%d [%s] %s",
		time.Now().Format("2006-01-02 15:04:05"), level, file, line, apiName, msg)
}

// caller 跳过 skip 层栈帧，返回文件名（短名）与行号（调用链：业务代码 → LogXxx → writeLog → caller）。
func caller(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "???", 0
	}
	return filepath.Base(file), line
}
