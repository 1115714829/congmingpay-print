package ui

import (
	"fmt"

	"congmingpay/internal/preview"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func (a *App) onPreviewJob() {
	j := a.selectedJob()
	if j == nil {
		a.warn("预览", "请先选择一个打印任务。")
		return
	}
	pd, err := a.svc.PreviewData(j.No)
	if err != nil {
		a.warn("预览", err.Error())
		return
	}
	if ok, reason := preview.CanPreview(pd); !ok {
		a.warn("预览", reason)
		return
	}
	img, err := preview.Render(preview.FromJob(pd))
	if err != nil {
		a.warn("预览", "渲染失败: "+err.Error())
		return
	}
	bmp, err := walk.NewBitmapFromImage(img)
	if err != nil {
		a.warn("预览", "创建位图失败: "+err.Error())
		return
	}

	title := fmt.Sprintf("预览 任务#%d · %s · %dmm", pd.JobNo, pd.Printer.Name, pd.WidthMM)
	note := "近似效果(机打二维码/条码/字体与真机可能略有差异)"
	if pd.Buzzer {
		note += " · 蜂鸣已开"
	}
	if pd.Cut {
		note += " · 切刀已开"
	}

	var dlg *walk.Dialog
	_, err = (Dialog{
		AssignTo: &dlg,
		Title:    title,
		MinSize:  Size{Width: 420, Height: 520},
		Size:     Size{Width: 480, Height: 640},
		Layout:   VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6},
		Children: []Widget{
			Label{Text: note},
			ScrollView{
				HorizontalFixed: true,
				Layout:          VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					ImageView{
						Mode:  ImageViewModeIdeal,
						Image: bmp,
					},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "关闭", OnClicked: func() { dlg.Accept() }},
				},
			},
		},
	}).Run(a.mw)
	bmp.Dispose()
	if err != nil {
		a.warn("预览", "打开预览窗口失败: "+err.Error())
	}
}
