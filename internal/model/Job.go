package model

// JobStatus 是打印任务状态。
type JobStatus string

const (
	JobQueued   JobStatus = "queued"
	JobPrinting JobStatus = "printing"
	JobWaiting  JobStatus = "waiting" // 网络异常,等待重试(堆在任务列表,网络恢复后自动打出)
	JobDone     JobStatus = "done"
	JobFailed   JobStatus = "failed"
)

// Label 返回任务状态的中文标签。
func (s JobStatus) Label() string {
	switch s {
	case JobQueued:
		return "排队中"
	case JobPrinting:
		return "打印中"
	case JobWaiting:
		return "等待重试"
	case JobDone:
		return "已完成"
	case JobFailed:
		return "失败"
	default:
		return string(s)
	}
}

// Active 表示任务是否仍在进行(排队 / 打印中 / 等待重试)。
func (s JobStatus) Active() bool {
	return s == JobQueued || s == JobPrinting || s == JobWaiting
}

// Job 是一次打印任务(用于展示)。
type Job struct {
	No        int
	Doc       string
	PrinterID string
	Printer   string
	Copies    int
	Status    JobStatus
	Time      string
	Err       string
}
