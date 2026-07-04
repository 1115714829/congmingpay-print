package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"
)

// PrinterInfo 打印机信息(GET 返回)。
type PrinterInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Brand string `json:"brand"`
	Width int    `json:"width"`
	Conn  string `json:"conn"`
	Addr  string `json:"addr"`
}

// PrinterSync 是云端下发的一台打印机定义(POST 同步用)。
//
// 用例(下发两台,一台网口带品牌、一台缺品牌→其他):
//
//	[
//	  {"conn":"ip","ip":"192.168.0.89","port":"9100","name":"厨房-01","brand":"佳博","width":80},
//	  {"conn":"ip","ip":"192.168.0.90","name":"前台-01","width":58}
//	]
type PrinterSync struct {
	// 连接方式:"ip"/"network"=网口,"usb"=USB(默认网口)。
	Conn string `json:"conn" example:"ip"`
	// 网口 IP(conn=ip 必填,作为身份匹配键)。
	IP string `json:"ip" example:"192.168.0.89"`
	// 网口端口(可选,默认 9100)。
	Port string `json:"port" example:"9100"`
	// USB 设备名(conn=usb 必填,作为身份匹配键)。
	USBName string `json:"usbName"`
	// 名称(可选;缺省用 IP/USB 名)。
	Name string `json:"name" example:"厨房-01"`
	// 品牌/型号(可选,缺省→其他):佳博 / 飞蛾 / 其他。
	Brand string `json:"brand" example:"佳博"`
	// 纸宽 mm:58 或 80(可选,默认 80)。
	Width int `json:"width" example:"80"`
}

// toPrinter 把下发定义转成 model.Printer(身份字段 + 展示字段)。
func (d PrinterSync) toPrinter() *model.Printer {
	p := &model.Printer{
		Name:    strings.TrimSpace(d.Name),
		Brand:   model.Brand(strings.TrimSpace(d.Brand)),
		Width:   d.Width,
		USBName: strings.TrimSpace(d.USBName),
	}
	if strings.EqualFold(strings.TrimSpace(d.Conn), "usb") {
		p.Conn = model.ConnUSB
	} else {
		p.Conn = model.ConnNetwork
		p.IP = strings.TrimSpace(d.IP)
		p.Port = strings.TrimSpace(d.Port)
	}
	if p.Name == "" {
		if p.Conn == model.ConnUSB {
			p.Name = p.USBName
		} else {
			p.Name = p.IP
		}
	}
	return p
}

// handlePrinters godoc
// @Summary     列出 / 同步打印机
// @Description GET:列出已注册打印机(返回 PrinterInfo 数组)。
// @Description POST:云端下发打印机清单(PrinterSync 数组),按身份(网口=IP,USB=usbName)增量 upsert——有则只更新展示字段、无则新建(替代手动创建);不传 brand→其他,不传 width→80;返回同步后的完整列表。
// @Description 常见错误码:400=POST body 不是打印机数组 / JSON 解析失败;405=非 GET/POST。
// @Tags        print
// @Accept      json
// @Produce     json
// @Param       request body []PrinterSync false "打印机清单(仅 POST)"
// @Success     200 {array} PrinterInfo "打印机列表(GET 现有;POST 同步后)"
// @Failure     400 {object} PrintResponse "POST body 应为打印机数组 / JSON 解析失败"
// @Failure     405 {object} PrintResponse "仅支持 GET/POST"
// @Router      /api/printers [get]
// @Router      /api/printers [post]
func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPrinters(w)
	case http.MethodPost:
		s.syncPrinters(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, PrintResponse{Message: "仅支持 GET/POST"})
	}
}

// listPrinters 返回已注册打印机列表。
func (s *Server) listPrinters(w http.ResponseWriter) {
	list := s.cfg.PrinterList()
	out := make([]PrinterInfo, 0, len(list))
	for _, p := range list {
		out = append(out, PrinterInfo{
			ID: p.ID, Name: p.Name, Brand: p.BrandLabel(),
			Width: p.Width, Conn: p.ConnLabel(), Addr: p.Target(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// syncPrinters 处理云端下发的打印机清单:逐个 upsert,保存并通知 UI 刷新。
func (s *Server) syncPrinters(w http.ResponseWriter, r *http.Request) {
	var defs []PrinterSync
	if err := json.NewDecoder(r.Body).Decode(&defs); err != nil {
		writeJSON(w, http.StatusBadRequest, PrintResponse{Message: "JSON 解析失败(应为打印机数组): " + err.Error()})
		return
	}
	added, updated := 0, 0
	for _, d := range defs {
		p := d.toPrinter()
		if p.Conn == model.ConnNetwork && p.IP == "" {
			continue
		}
		if p.Conn == model.ConnUSB && p.USBName == "" {
			continue
		}
		if _, isNew := s.cfg.UpsertPrinter(p); isNew {
			added++
		} else {
			updated++
		}
	}
	s.save()
	s.fireChange()
	logger.Infof("云端下发打印机 %d 台:新增 %d,更新 %d", len(defs), added, updated)
	s.listPrinters(w)
}
