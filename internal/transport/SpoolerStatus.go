//go:build windows

package transport

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PRINTER_INFO_2 的 Status / Attributes 位(仅列用到的)。
const (
	printerStatusError        = 0x00000002
	printerStatusPaperJam     = 0x00000008
	printerStatusPaperOut     = 0x00000010
	printerStatusOffline      = 0x00000080
	printerStatusNotAvailable = 0x00001000
	printerStatusDoorOpen     = 0x00400000
	printerAttrWorkOffline    = 0x00000400
)

var (
	modWinspool      = windows.NewLazySystemDLL("winspool.drv")
	procOpenPrinter  = modWinspool.NewProc("OpenPrinterW")
	procGetPrinter   = modWinspool.NewProc("GetPrinterW")
	procClosePrinter = modWinspool.NewProc("ClosePrinter")
)

// printerInfo2 对应 Win32 PRINTER_INFO_2;指针字段用 uintptr 占位,只读取 Attributes/Status。
// 字段顺序与 C 结构一致,在 386 与 amd64 上布局均正确。
type printerInfo2 struct {
	serverName         uintptr
	printerName        uintptr
	shareName          uintptr
	portName           uintptr
	driverName         uintptr
	comment            uintptr
	location           uintptr
	devMode            uintptr
	sepFile            uintptr
	printProcessor     uintptr
	datatype           uintptr
	parameters         uintptr
	securityDescriptor uintptr
	Attributes         uint32
	Priority           uint32
	DefaultPriority    uint32
	StartTime          uint32
	UntilTime          uint32
	Status             uint32
	CJobs              uint32
	AveragePPM         uint32
}

// QuerySpoolerStatus 经 Windows 打印后台(GetPrinter)查询已装打印机状态。
// 注:普通 USB RAW 驱动多为单向,后台常只反映"离线/暂停/工作离线"等,
// 缺纸/开盖等物理状态需驱动支持双向通信才会体现。
func QuerySpoolerStatus(name string) (PrinterStatus, error) {
	attrs, status, err := getPrinterStatus(name)
	if err != nil {
		return PrinterStatus{}, err
	}
	offline := status&printerStatusOffline != 0 ||
		status&printerStatusNotAvailable != 0 ||
		attrs&printerAttrWorkOffline != 0
	s := PrinterStatus{
		Reachable: true,
		Online:    !offline,
		PaperOut:  status&printerStatusPaperOut != 0,
		CoverOpen: status&printerStatusDoorOpen != 0,
		Error:     status&(printerStatusError|printerStatusPaperJam) != 0,
	}
	s.Detail = summarizeStatus(s)
	if s.Online && status == 0 {
		s.Detail += "(普通USB驱动可能仅反映后台状态,物理缺纸/开盖未必上报)"
	}
	return s, nil
}

func getPrinterStatus(name string) (attributes, status uint32, err error) {
	pname, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, 0, err
	}
	var h windows.Handle
	r, _, e := procOpenPrinter.Call(uintptr(unsafe.Pointer(pname)), uintptr(unsafe.Pointer(&h)), 0)
	if r == 0 {
		return 0, 0, fmt.Errorf("OpenPrinter %q 失败: %v", name, e)
	}
	defer procClosePrinter.Call(uintptr(h))

	var needed uint32
	procGetPrinter.Call(uintptr(h), 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return 0, 0, fmt.Errorf("GetPrinter 返回所需大小为 0")
	}
	buf := make([]byte, needed)
	r, _, e = procGetPrinter.Call(uintptr(h), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return 0, 0, fmt.Errorf("GetPrinter %q 失败: %v", name, e)
	}
	pi := (*printerInfo2)(unsafe.Pointer(&buf[0]))
	return pi.Attributes, pi.Status, nil
}
