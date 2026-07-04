package ui

import (
	"strconv"

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
		return strconv.Itoa(j.Copies)
	case 4:
		return j.Status.Label()
	case 5:
		return j.Time
	}
	return ""
}

// StyleCell 给「状态」列按任务状态上色。
func (m *JobModel) StyleCell(style *walk.CellStyle) {
	if style.Col() != 4 || style.Row() < 0 || style.Row() >= len(m.items) {
		return
	}
	switch m.items[style.Row()].Status {
	case model.JobPrinting:
		style.TextColor = colBlue
	case model.JobQueued, model.JobWaiting:
		style.TextColor = colOrange
	case model.JobDone:
		style.TextColor = colGreen
	case model.JobFailed:
		style.TextColor = colRed
	}
}
