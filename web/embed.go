// Package web 内嵌前端静态资源（Vue 3 + Element Plus 本地 vendor，无 CDN/构建依赖）。
package web

import "embed"

//go:embed index.html js vendor favicon.ico favicon.png favicon.svg
var FS embed.FS
