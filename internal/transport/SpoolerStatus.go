//go:build windows

package transport

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PRINTER_INFO_2 的 Status / Attributes 位(仅列用到的)。
const (
	printerStatusOffline      = 0x00000080
	printerStatusNotAvailable = 0x00001000
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

// QuerySpoolerStatus 经 Windows 打印后台(GetPrinter)查询已装打印机的在线/离线。
// 与网口一致仅提供在线/离线两态:程序转述打印后台的脱机标志
// (PRINTER_STATUS_OFFLINE / NOT_AVAILABLE / WORK_OFFLINE)。
func QuerySpoolerStatus(name string) (PrinterStatus, error) {
	attrs, status, err := getPrinterStatus(name)
	if err != nil {
		return PrinterStatus{}, err
	}
	offline := status&printerStatusOffline != 0 ||
		status&printerStatusNotAvailable != 0 ||
		attrs&printerAttrWorkOffline != 0
	s := PrinterStatus{Reachable: true, Online: !offline}
	if offline {
		s.Detail = "打印后台标记脱机"
	} else {
		s.Detail = "打印后台正常"
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
