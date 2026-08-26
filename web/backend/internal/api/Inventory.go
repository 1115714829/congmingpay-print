package api

import (
	"errors"
	"net/http"

	"congmingpay/web/internal/inventory"
	"congmingpay/web/internal/store"
)

func (s *Server) hImport(w http.ResponseWriter, r *http.Request) {
	// multipart 文件字段 file;上限 10MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeErr(w, http.StatusOK, CodeFileRead, "文件读取失败")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil || header == nil {
		writeErr(w, http.StatusOK, CodeFileRead, "文件读取失败")
		return
	}
	defer file.Close()
	res, err := inventory.ImportCSV(s.Store, file)
	if err != nil {
		var dup *inventory.DupError
		if errors.As(err, &dup) {
			code := CodeDupInStore
			if dup.Code == 4003 {
				code = CodeDupInFile
			}
			writeErr(w, http.StatusOK, code, err.Error())
			return
		}
		msg := err.Error()
		code := CodeTmplBad
		switch {
		case msg == "文件读取失败: 不是合法 CSV":
			code = CodeFileRead
			msg = "文件读取失败"
		case msg == "模板格式错误: 空文件" || msg == "模板格式错误: 首行表头必须为 DeviceName,DeviceSecret,ProductKey" ||
			msg == "模板格式错误: 缺少数据行":
			code = CodeTmplBad
		}
		writeErr(w, http.StatusOK, code, msg)
		return
	}
	writeOK(w, map[string]interface{}{"imported": res.Imported, "total": res.Total})
}

func (s *Server) hInventory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	total, list, err := s.Store.ListDevices(
		atoiParam(q.Get("page")), atoiParam(q.Get("pageSize")),
		"", store.StateInventory, 0)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	writeOK(w, map[string]interface{}{"total": total, "items": deviceVOs(list)})
}
