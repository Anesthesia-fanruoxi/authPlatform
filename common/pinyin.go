package common

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// Pinyin 返回昵称的完整拼音串（无分隔、无声调、小写）；非中文（ASCII/数字/符号）原样保留。
// 示例："测试" → "ceshi"；"abc张" → "abczhang"；"" → ""
func Pinyin(s string) string {
	if s == "" {
		return ""
	}
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal // 完整拼音（无声调）
	var b strings.Builder
	for _, r := range s {
		// LazyPinyin 对单字返回其拼音（无拼音的非中文字符返回空切片），非中文原样保留
		seg := pinyin.LazyPinyin(string(r), args)
		if len(seg) > 0 {
			b.WriteString(seg[0])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
