package ui

import (
	"strconv"

	"congmingpay/internal/model"

	. "github.com/lxn/walk/declarative"
)

func (a *App) jobsPageWidget() Composite {
	// 构建即隐藏:walk 创建窗口时布局最小尺寸按全部可见子控件计算,非首屏页若可见
	// 会把最小高度叠加进去,导致窗口被撑大且之后不缩回(showPage 切页时才改可见性)。
	return Composite{
		AssignTo: &a.jobsPage,
		Visible:  false,
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				Layout: HBox{Margins: Margins{Left: 6, Top: 4, Right: 6, Bottom: 4}, Spacing: 4},
				Children: []Widget{
					PushButton{Text: "预览", OnClicked: a.onPreviewJob},
					PushButton{Text: "重新打印", OnClicked: a.onRetryJob},
					PushButton{Text: "取消", OnClicked: a.onCancelJob},
					PushButton{Text: "清除已完成", OnClicked: a.onClearDone},
					HSpacer{},
				},
			},
			TableView{
				AssignTo:            &a.jobTV,
				Model:               a.jobModel,
				StyleCell:           a.jobModel.StyleCell,
				LastColumnStretched: true, // 窗口放大时末列(详情)吃掉剩余宽度,表格填满
				// 列宽合计 661 ≤ 686 内容宽;表格最小宽=列宽合计,超了会把主窗口撑大
				Columns: []TableViewColumn{
					{Title: "任务号", Width: 56},
					{Title: "文档", Width: 120},
					{Title: "目标打印机", Width: 96},
					{Title: "状态", Width: 64},
					{Title: "提交时间", Width: 74},
					{Title: "详情", Width: 251},
				},
			},
		},
	}
}

func (a *App) refreshJobs() {
	a.jobModel.items = a.svc.Jobs()
	publishTableRefresh(&a.jobModel.TableModelBase, len(a.jobModel.items))
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
	// 任务重新入队,旧告警作废(再失败/再卡单会重新挂起)
	a.alertResolve(alertJobFailed, strconv.Itoa(j.No))
	a.alertResolve(alertJobWaiting, strconv.Itoa(j.No))
	a.flash("任务 #" + strconv.Itoa(j.No) + " 已重新加入队列")
}

func (a *App) onCancelJob() {
	j := a.selectedJob()
	if j == nil {
		a.warn("打印队列", "请先选择一个打印任务。")
		return
	}
	a.svc.Cancel(j.No)
	// 取消的任务不保留告警行
	a.alertResolve(alertJobFailed, strconv.Itoa(j.No))
	a.alertResolve(alertJobWaiting, strconv.Itoa(j.No))
	a.refreshJobs()
	a.flash("已取消任务 #" + strconv.Itoa(j.No))
}

func (a *App) onClearDone() {
	a.svc.ClearDone()
	a.refreshJobs()
	a.flash("已清除已完成任务")
}
