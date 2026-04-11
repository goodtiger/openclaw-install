// Package output 提供统一的错误、警告和成功消息输出函数，
// 支持附带可操作的修复建议（FixableError）。
package output

import (
	"errors"
	"fmt"
	"io"
)

// FixableError 是携带修复建议的错误类型。
// 当错误可以通过具体操作修复时，使用此类型替代普通 error。
type FixableError struct {
	Msg   string // 错误描述
	Fix   string // 修复建议
	Cause error  // 底层错误（可选）
}

func (e *FixableError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *FixableError) Unwrap() error {
	return e.Cause
}

// NewFixable 创建携带修复建议的错误。
func NewFixable(msg, fix string) *FixableError {
	return &FixableError{Msg: msg, Fix: fix}
}

// NewFixablef 创建携带修复建议的格式化错误。
func NewFixablef(msg, fix string, args ...any) *FixableError {
	return &FixableError{Msg: fmt.Sprintf(msg, args...), Fix: fix}
}

// WrapFixable 将已有错误包装为带修复建议的错误。
func WrapFixable(cause error, msg, fix string) *FixableError {
	return &FixableError{Msg: msg, Fix: fix, Cause: cause}
}

// ErrorWithFix 输出错误消息和修复建议到 errOut。
// 格式：
//
//	❌ <msg>
//	💡 <fix>
func ErrorWithFix(errOut io.Writer, msg, fix string) {
	fmt.Fprintf(errOut, "\u274c %s\n", msg)
	fmt.Fprintf(errOut, "\U0001f4a1 %s\n", fix)
}

// ErrorWithFixf 格式化输出错误消息和修复建议。
func ErrorWithFixf(errOut io.Writer, msg, fix string, args ...any) {
	fmt.Fprintf(errOut, "\u274c %s\n", fmt.Sprintf(msg, args...))
	fmt.Fprintf(errOut, "\U0001f4a1 %s\n", fix)
}

// PrintError 输出普通错误（无修复建议）。
func PrintError(errOut io.Writer, msg string) {
	fmt.Fprintf(errOut, "\u274c %s\n", msg)
}

// PrintWarn 输出警告消息。
func PrintWarn(out io.Writer, msg string) {
	fmt.Fprintf(out, "\u26a0\ufe0f %s\n", msg)
}

// PrintSuccess 输出成功消息。
func PrintSuccess(out io.Writer, msg string) {
	fmt.Fprintf(out, "\u2705 %s\n", msg)
}

// FindFixableError 在错误链中查找 FixableError。
// 如果找到，返回该错误和 true；否则返回 nil, false。
func FindFixableError(err error) (*FixableError, bool) {
	if err == nil {
		return nil, false
	}
	var fixable *FixableError
	if errors.As(err, &fixable) {
		return fixable, true
	}
	return nil, false
}

// PrintErrorWithFix 智能打印错误：如果错误链中包含 FixableError，
// 则输出错误消息和修复建议；否则只输出原始错误。
func PrintErrorWithFix(errOut io.Writer, err error) {
	if fixable, ok := FindFixableError(err); ok {
		ErrorWithFix(errOut, fixable.Error(), fixable.Fix)
	} else {
		PrintError(errOut, err.Error())
	}
}
