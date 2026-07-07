package ui

import (
	"strconv"

	"congmingpay/internal/model"

	. "github.com/lxn/walk/declarative"
)

func (a *App) jobsPageWidget() Composite {
	return Composite{
		AssignTo: &a.jobsPage,
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				Layout: HBox{Margins: Margins{Left: 6, Top: 4, Right: 6, Bottom: 4}, Spacing: 4},
				Children: []Widget{
					PushButton{Text: "重新打印", OnClicked: a.onRetryJob},
					PushButton{Text: "取消", OnClicked: a.onCancelJob},
					PushButton{Text: "清除已完成", OnClicked: a.onClearDone},
					HSpacer{},
				},
			},
			TableView{
				AssignTo:  &a.jobTV,
				Model:     a.jobModel,
				StyleCell: a.jobModel.StyleCell,
				Columns: []TableViewColumn{
					{Title: "任务号", Width: 70},
					{Title: "文档", Width: 170},
					{Title: "目标打印机", Width: 130},
					{Title: "份数", Width: 50},
					{Title: "状态", Width: 80},
					{Title: "提交时间", Width: 90},
					{Title: "详情", Width: 280},
				},
			},
		},
	}
}

func (a *App) refreshJobs() {
	a.jobModel.items = a.svc.Jobs()
	a.jobModel.PublishRowsReset()
}

func (a *App) selectedJob() *model.Job {
	if a.jobTV == nil {
		return nil
	}
	i := a.jobTV.CurrentIndex()
	if i < 0 || i >= len(a.jobModel.items) {
		return nil
	}
	return a.jobModel.items[i]
}

func (a *App) onRetryJob() {
	j := a.selectedJob()
	if j == nil {
		a.warn("打印队列", "请先选择一个打印任务。")
		return
	}
	a.svc.Retry(j.No)
	a.flash("任务 #" + strconv.Itoa(j.No) + " 已重新加入队列")
}

func (a *App) onCancelJob() {
	j := a.selectedJob()
	if j == nil {
		a.warn("打印队列", "请先选择一个打印任务。")
		return
	}
	a.svc.Cancel(j.No)
	a.refreshJobs()
	a.flash("已取消任务 #" + strconv.Itoa(j.No))
}

func (a *App) onClearDone() {
	a.svc.ClearDone()
	a.refreshJobs()
	a.flash("已清除已完成任务")
}
