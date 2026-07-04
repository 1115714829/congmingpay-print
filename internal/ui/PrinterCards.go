package ui

import (
	"fmt"

	"congmingpay/internal/logger"
	"congmingpay/internal/model"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// 统一按钮样式(左侧导航与顶部工具栏共用):同高度、同字体(默认字体)。
const btnHeight = 30

var btnFont = Font{} // 空=默认字体,只统一高度即可

// 卡片尺寸与配色。
const (
	cardWidth  = 172
	cardHeight = 112
)

var (
	cardColSel = walk.RGB(0xcc, 0xe4, 0xf7) // 选中卡浅蓝底
	cardColBg  = walk.RGB(0xff, 0xff, 0xff) // 普通卡白底
)

// printerCard 持有一张卡片的控件引用,便于就地更新(不整体重建)。
type printerCard struct {
	id          string
	root        *walk.Composite
	ipLabel     *walk.Label
	statusLabel *walk.Label
	sourceLabel *walk.Label
	queueLabel  *walk.Label
	nameLabel   *walk.Label
}

// refreshCards 刷新打印机卡片:ID 集合变了就重建,否则就地更新状态/队列/高亮(不闪)。
func (a *App) refreshCards() {
	if a.cardsView == nil {
		return
	}
	printers := a.filteredPrinters()
	ids := make([]string, len(printers))
	for i, p := range printers {
		ids[i] = p.ID
	}
	if !sameIDs(ids, a.cardOrder) {
		a.rebuildCards(printers)
	} else {
		for _, p := range printers {
			if c := a.cards[p.ID]; c != nil {
				a.updateCard(c, p)
			}
		}
	}
	a.updateStatusBar()
}

func sameIDs(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// rebuildCards 清空并重建所有卡片(打印机列表增删/筛选变化时)。
func (a *App) rebuildCards(printers []*model.Printer) {
	for _, c := range a.cards {
		c.root.Dispose()
	}
	a.cards = map[string]*printerCard{}
	a.cardOrder = a.cardOrder[:0]
	if a.selectedID != "" && a.cfg.FindPrinter(a.selectedID) == nil {
		a.selectedID = "" // 选中项已被删/筛掉
	}
	builder := NewBuilder(a.cardsView)
	for _, p := range printers {
		c := &printerCard{id: p.ID}
		id := p.ID
		click := func(x, y int, button walk.MouseButton) { a.selectCard(id) }
		err := (Composite{
			AssignTo:    &c.root,
			Border:      true,
			MinSize:     Size{Width: cardWidth, Height: cardHeight},
			MaxSize:     Size{Width: cardWidth, Height: cardHeight},
			Layout:      VBox{Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}, Spacing: 1},
			OnMouseDown: click,
			Children: []Widget{
				Label{AssignTo: &c.ipLabel, Font: Font{PointSize: 10, Bold: true}, OnMouseDown: click},
				Label{AssignTo: &c.statusLabel, OnMouseDown: click},
				Label{AssignTo: &c.sourceLabel, OnMouseDown: click},
				Label{AssignTo: &c.queueLabel, OnMouseDown: click},
				VSpacer{},
				Label{AssignTo: &c.nameLabel, TextColor: colGray, OnMouseDown: click},
			},
		}).Create(builder)
		if err != nil {
			logger.Errorf("建卡片失败: %v", err)
			continue
		}
		a.cards[p.ID] = c
		a.cardOrder = append(a.cardOrder, p.ID)
		a.updateCard(c, p)
	}
}

// updateCard 就地更新一张卡片的文字/颜色/高亮。
func (a *App) updateCard(c *printerCard, p *model.Printer) {
	head := "IP:" + p.IP
	if p.Conn == model.ConnUSB {
		head = "USB:" + p.USBName
	}
	c.ipLabel.SetText(head)

	st, ok := a.pstatus[p.ID]
	if !ok {
		st = statusInfo{label: "—", color: colGray}
	}
	c.statusLabel.SetText("状态:" + st.label)
	c.statusLabel.SetTextColor(st.color)

	c.sourceLabel.SetText("来源:" + p.SourceLabel())

	active, printing := a.svc.PrinterQueue(p.ID)
	state := "空闲中"
	if printing {
		state = "打印中"
	} else if active > 0 {
		state = "排队中"
	}
	c.queueLabel.SetText(fmt.Sprintf("队列:%d/%s", active, state))

	c.nameLabel.SetText("名称:" + p.Name)

	a.applyCardHighlight(c)
}

// selectCard 选中一张卡片并刷新所有卡片高亮。
func (a *App) selectCard(id string) {
	if a.selectedID == id {
		return
	}
	a.selectedID = id
	for _, c := range a.cards {
		a.applyCardHighlight(c)
	}
}

// applyCardHighlight 按是否选中设置卡片背景(选中浅蓝、其余白)。
func (a *App) applyCardHighlight(c *printerCard) {
	if a.cardBrushSel == nil {
		a.cardBrushSel, _ = walk.NewSolidColorBrush(cardColSel)
		a.cardBrushNormal, _ = walk.NewSolidColorBrush(cardColBg)
	}
	if c.id == a.selectedID {
		c.root.SetBackground(a.cardBrushSel)
	} else {
		c.root.SetBackground(a.cardBrushNormal)
	}
}
