package ui

import (
	"strconv"
	"strings"

	"congmingpay/internal/model"
	"congmingpay/internal/transport"

	"github.com/lxn/walk"
)

// 设计稿状态配色。
var (
	colGreen  = walk.RGB(0x10, 0x7c, 0x10)
	colGray   = walk.RGB(0x61, 0x61, 0x61)
	colOrange = walk.RGB(0x9d, 0x5d, 0x00)
	colRed    = walk.RGB(0xc4, 0x2b, 0x1c)
	colBlue   = walk.RGB(0x00, 0x67, 0xc0)
)

type statusInfo struct {
	label  string
	color  walk.Color
	detail string // 传输层原始描述(如"在线(ping 5ms)"/"离线(超时)"),供日志
}

// statusInfoFor 把传输层状态映射为短标签 + 颜色(+ 原始描述)。
func statusInfoFor(st transport.PrinterStatus) statusInfo {
	switch {
	case !st.Reachable || !st.Online:
		return statusInfo{"离线", colGray, st.Detail}
	case st.PaperOut:
		return statusInfo{"缺纸", colOrange, st.Detail}
	case st.Error || st.CoverOpen:
		return statusInfo{"错误", colRed, st.Detail}
	default:
		return statusInfo{"就绪", colGreen, st.Detail}
	}
}

// publishTableRefresh 全量刷新 TableView 并强制重绘可见行。
// 坑:walk 的 PublishRowsReset 底层 LVM_SETITEMCOUNT 带 LVSICF_NOINVALIDATEALL——
// 行数不变时 listview 不重绘任何行(旧文本滞留屏幕,靠点击/遮挡才偶然刷新);
// 必须补发 PublishRowsChanged(0,n-1) 走 LVM_REDRAWITEMS 强制重绘。勿裸调 PublishRowsReset。
func publishTableRefresh(base *walk.TableModelBase, rowCount int) {
	base.PublishRowsReset()
	if rowCount > 0 {
		base.PublishRowsChanged(0, rowCount-1)
	}
}

// PrinterModel 是打印机列表的 TableView 模型。
type PrinterModel struct {
	walk.TableModelBase
	items  []*model.Printer
	status map[string]statusInfo
}

func newPrinterModel() *PrinterModel {
	return &PrinterModel{status: map[string]statusInfo{}}
}

func (m *PrinterModel) RowCount() int { return len(m.items) }

func (m *PrinterModel) Value(row, col int) interface{} {
	p := m.items[row]
	switch col {
	case 0:
		return p.Name
	case 1:
		return p.BrandLabel()
	case 2:
		return p.WidthLabel()
	case 3:
		return p.ConnLabel()
	case 4:
		return p.Address()
	case 5:
		return m.info(p).label
	case 6:
		return p.LastPrint
	}
	return ""
}

func (m *PrinterModel) info(p *model.Printer) statusInfo {
	if s, ok := m.status[p.ID]; ok {
		return s
	}
	return statusInfo{"—", colGray, ""}
}

// StyleCell 给「状态」列上色。
func (m *PrinterModel) StyleCell(style *walk.CellStyle) {
	if style.Col() != 5 || style.Row() < 0 || style.Row() >= len(m.items) {
		return
	}
	style.TextColor = m.info(m.items[style.Row()]).color
}

// JobModel 是打印队列的 TableView 模型。
type JobModel struct {
	walk.TableModelBase
	items []*model.Job
}

func newJobModel() *JobModel { return &JobModel{} }

func (m *JobModel) RowCount() int { return len(m.items) }

func (m *JobModel) Value(row, col int) interface{} {
	j := m.items[row]
	switch col {
	case 0:
		return "#" + strconv.Itoa(j.No)
	case 1:
		return j.Doc
	case 2:
		return j.Printer
	case 3:
		return j.Status.Label()
	case 4:
		return j.Time
	case 5:
		return j.Err // 详情:等待原因 / 「长期等待·疑似端口未就绪/故障」告警 / 失败原因
	}
	return ""
}

// StyleCell 给「状态」列与「详情」列上色(长期等待告警醒目红)。
func (m *JobModel) StyleCell(style *walk.CellStyle) {
	row := style.Row()
	if row < 0 || row >= len(m.items) {
		return
	}
	j := m.items[row]
	switch style.Col() {
	case 3: // 状态
		switch j.Status {
		case model.JobPrinting:
			style.TextColor = colBlue
		case model.JobQueued, model.JobWaiting:
			style.TextColor = colOrange
		case model.JobDone:
			style.TextColor = colGreen
		case model.JobFailed:
			style.TextColor = colRed
		}
	case 5: // 详情:长期等待告警 / 失败 → 红;普通等待 → 橙
		if j.Status == model.JobFailed || (j.Status == model.JobWaiting && strings.Contains(j.Err, "疑似")) {
			style.TextColor = colRed
		} else if j.Status == model.JobWaiting {
			style.TextColor = colOrange
		}
	}
}
