package printsvc

import (
	"fmt"

	"congmingpay/internal/model"
)

// JobPreview 是供 UI 出图预览的只读快照(不含 ESC data_blob)。
type JobPreview struct {
	JobNo       int
	Doc         string
	Printer     model.Printer
	WidthMM     int
	ContentType int // ContentJSON / ContentESC / ContentUnknown
	SourceJSON  []byte
	Cut         bool
	Buzzer      bool
	HeadLines   int
	TailLines   int
	Reprint     bool
}

// PreviewData 返回任务预览所需字段。任务不存在时返回错误。
func (s *Service) PreviewData(jobNo int) (*JobPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.find(jobNo)
	if e == nil {
		return nil, fmt.Errorf("任务 #%d 不存在", jobNo)
	}
	w := e.printer.Width
	if w != 58 && w != 80 {
		w = 80
	}
	out := &JobPreview{
		JobNo: e.job.No, Doc: e.job.Doc, Printer: e.printer, WidthMM: w,
		ContentType: e.contentType,
		Cut:         e.cut, Buzzer: e.buzzer,
		HeadLines: e.headLines, TailLines: e.tailLines, Reprint: e.reprintNext,
	}
	if len(e.sourceJSON) > 0 {
		out.SourceJSON = append([]byte(nil), e.sourceJSON...)
	}
	return out, nil
}
