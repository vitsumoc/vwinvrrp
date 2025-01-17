package main

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
)

var RunLogger *lumberjack.Logger
var VRunLogLevel RunLogLevel = RL_INFO

// 运行日志初始化
func InitRunLog() {
	RunLogger = &lumberjack.Logger{
		Filename:   "./logs/vrrp.log",
		MaxSize:    50,    // mb
		MaxBackups: 20,    // 最大文件保留
		MaxAge:     30,    // 最大天数保留
		LocalTime:  false, // 使用UTC时间
		Compress:   false, // 不压缩
	}
	RunLog(RL_INFO, "运行日志初始化...")
}

// 日志等级
type RunLogLevel int

const (
	RL_DEBUG RunLogLevel = iota
	RL_INFO
	RL_ERROR
)

// 写入日志
func RunLog(level RunLogLevel, format string, a ...any) {
	if level < VRunLogLevel {
		return
	}
	// 获取当前时间
	now := time.Now()
	timeStr := fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d.%03d] ",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
		now.Nanosecond()/1000000)
	// 日志等级
	var levelStr string
	switch level {
	case RL_DEBUG:
		levelStr = "[DEBUG] "
	case RL_INFO:
		levelStr = "[INFO] "
	case RL_ERROR:
		levelStr = "[ERROR] "
	}
	// 写入日志
	RunLogger.Write([]byte(timeStr + levelStr + fmt.Sprintf(format, a...) + "\n"))
	fmt.Println(timeStr + levelStr + fmt.Sprintf(format, a...))
}

// WEB恢复
func WebRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录错误信息，可以使用自己喜欢的日志库
				RunLog(RL_ERROR, "发生panic:\n%v", err)
				stack := debug.Stack()
				RunLog(RL_ERROR, "调用栈信息:\n%s", stack)
				// 设置响应状态码为500
				c.JSON(http.StatusInternalServerError, gin.H{"error": "内部服务器错误"})
			}
		}()
		// 继续执行下一个中间件或路由处理函数
		c.Next()
	}
}

// Panic 记录
func LogPanic() {
	stack := debug.Stack()
	RunLog(RL_ERROR, "主程序异常退出")
	RunLog(RL_ERROR, "调用栈信息:\n%s", stack)
}
