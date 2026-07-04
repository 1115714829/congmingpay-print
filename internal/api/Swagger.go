package api

import (
	_ "embed"
	"net/http"
)

// swaggerJSON 由 swag CLI 从注解生成(见 build.ps1 的 swag init);内嵌以避免引入 swag 运行时库(保 go1.20 兼容)。
//
//go:embed docs/swagger.json
var swaggerJSON []byte

// swaggerUIHTML 是加载 Swagger UI 的页面(UI 资源走 CDN,规范从本地 /swagger/doc.json 取)。
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="zh"><head><meta charset="utf-8"><title>聪明付 打印服务 API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.onload=function(){SwaggerUIBundle({url:"/swagger/doc.json",dom_id:"#swagger-ui"});};</script>
</body></html>`

// mountSwagger 挂载 Swagger:/swagger/ 展示 UI,/swagger/doc.json 返回规范。
func (s *Server) mountSwagger(mux *http.ServeMux) {
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(swaggerJSON)
	})
	mux.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerUIHTML))
	})
}
