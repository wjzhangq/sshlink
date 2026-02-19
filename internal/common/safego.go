package common

import (
	"fmt"
	"runtime/debug"
)

// SafeGo 安全地启动 goroutine，捕获 panic 并记录堆栈
// 确保单个 goroutine 的 panic 不会导致整个程序崩溃
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Error("panic recovered: %v\n%s", r, debug.Stack())
			}
		}()
		fn()
	}()
}

// SafeGoWithName 安全地启动带名称的 goroutine，便于调试
func SafeGoWithName(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Error("panic recovered in %s: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// Recover 用于在 defer 中恢复 panic
// 使用示例:
//   defer common.Recover("functionName")
func Recover(name string) {
	if r := recover(); r != nil {
		Error("panic recovered in %s: %v\n%s", name, r, debug.Stack())
	}
}

// RecoverWithCallback 用于在 defer 中恢复 panic，并执行回调函数
// 使用示例:
//   defer common.RecoverWithCallback("functionName", func(err error) {
//       // 清理资源
//   })
func RecoverWithCallback(name string, callback func(err error)) {
	if r := recover(); r != nil {
		err := fmt.Errorf("panic in %s: %v", name, r)
		Error("%v\n%s", err, debug.Stack())
		if callback != nil {
			callback(err)
		}
	}
}
