//go:build windows

// Collect.go 采集本机硬件指纹(WMI,纯 Go 无 CGO,Win7 可用)。
package fingerprint

import (
	"fmt"
	"strings"

	"github.com/StackExchange/wmi"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Collect 采集本机硬件指纹:主板序列号 / CPU ID / 系统盘序列号。
// 不收网卡 MAC(网卡不稳定、USB 网卡干扰);只收系统盘(避免多盘干扰)。
// 系统盘定位按 1→2→3 依次回退(兼容不同系统 WMI 差异),全部为空时报错。
func Collect() (Fingerprint, error) {
	f := Fingerprint{OsType: "win"}

	var boards []struct {
		SerialNumber string `wmi:"SerialNumber"`
	}
	if err := wmi.Query("SELECT SerialNumber FROM Win32_BaseBoard", &boards); err == nil && len(boards) > 0 {
		f.BoardSerial = strings.TrimSpace(boards[0].SerialNumber)
	}

	var cpus []struct {
		ProcessorId string `wmi:"ProcessorId"`
	}
	if err := wmi.Query("SELECT ProcessorId FROM Win32_Processor", &cpus); err == nil && len(cpus) > 0 {
		f.CpuID = strings.TrimSpace(cpus[0].ProcessorId)
	}

	if s := systemDiskSerial(); s != "" {
		f.DiskSerials = []string{s}
	}

	if err := f.Validate(); err != nil {
		return Fingerprint{}, fmt.Errorf("硬件指纹采集失败:主板序列号与系统盘序列号均为空(%v)", err)
	}
	return f, nil
}

// systemDiskSerial 定位系统所在物理盘并返回其序列号,三级回退:
// 1. Win32_OperatingSystem.SystemDeviceName 直查;
// 2. 关联类表 C:→Win32_LogicalPartition→Win32_DiskDrive;
// 3. 系统盘卷序列号(GetVolumeInformation,仅兜底)。
func systemDiskSerial() string {
	if s := serialBySystemDeviceName(); s != "" {
		return s
	}
	if s := serialByPartitionChain(); s != "" {
		return s
	}
	return systemVolumeSerial()
}

// serialBySystemDeviceName 经 Win32_OperatingSystem.SystemDeviceName 定位(如 \\.\PHYSICALDRIVE0)。
func serialBySystemDeviceName() string {
	var oss []struct {
		SystemDeviceName string `wmi:"SystemDeviceName"`
	}
	if err := wmi.Query("SELECT SystemDeviceName FROM Win32_OperatingSystem", &oss); err != nil || len(oss) == 0 {
		return ""
	}
	return diskSerialByDeviceID(strings.TrimSpace(oss[0].SystemDeviceName))
}

// serialByPartitionChain 经关联类表定位:
// C: →(Win32_LogicalDiskToPartitionLine)Win32_LogicalPartition →(Win32_DiskPartitionToDiskDrive)Win32_DiskDrive。
func serialByPartitionChain() string {
	drive := systemDriveLetter()
	if drive == "" {
		return ""
	}
	var lines []struct {
		Group  string `wmi:"GroupComponent"`
		Result string `wmi:"ResultComponent"`
	}
	if err := wmi.Query("SELECT GroupComponent, ResultComponent FROM Win32_LogicalDiskToPartitionLine", &lines); err != nil {
		return ""
	}
	want := `Win32_LogicalDisk.DeviceID="` + drive + `"`
	var partPath string
	for _, l := range lines {
		if strings.HasSuffix(l.Group, want) {
			partPath = l.Result
			break
		}
	}
	if partPath == "" {
		return ""
	}
	var links []struct {
		Group  string `wmi:"GroupComponent"`
		Result string `wmi:"ResultComponent"`
	}
	if err := wmi.Query("SELECT GroupComponent, ResultComponent FROM Win32_DiskPartitionToDiskDrive", &links); err != nil {
		return ""
	}
	for _, l := range links {
		if strings.HasSuffix(l.Group, partPath) {
			return diskSerialByDeviceID(componentValue(l.Result, "DeviceID"))
		}
	}
	return ""
}

// diskSerialByDeviceID 按物理盘 DeviceID(如 \\.\PHYSICALDRIVE0)查序列号。
func diskSerialByDeviceID(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ""
	}
	var dds []struct {
		DeviceID     string `wmi:"DeviceID"`
		SerialNumber string `wmi:"SerialNumber"`
	}
	if err := wmi.Query("SELECT DeviceID, SerialNumber FROM Win32_DiskDrive", &dds); err != nil {
		return ""
	}
	for _, d := range dds {
		if strings.EqualFold(strings.TrimSpace(d.DeviceID), deviceID) {
			return strings.TrimSpace(d.SerialNumber)
		}
	}
	return ""
}

// componentValue 从 WMI 组件路径(如 X:Win32_DiskDrive.DeviceID="\\.\PHYSICALDRIVE0")取属性值。
func componentValue(path, key string) string {
	mk := `"` + key + `="`
	i := strings.LastIndex(path, mk)
	if i < 0 {
		return ""
	}
	rest := path[i+len(mk):]
	if j := strings.Index(rest, "\""); j >= 0 {
		return rest[:j]
	}
	return rest
}

// systemDriveLetter 返回系统盘盘符(如 "C:"),取自系统目录。
func systemDriveLetter() string {
	sysDir, err := windows.GetSystemDirectory()
	if err != nil || len(sysDir) < 2 {
		return ""
	}
	return string(sysDir[0]) + ":"
}

// systemVolumeSerial 读系统盘卷序列号(8 位十六进制),仅三级兜底使用
// (卷序列号格式化后会变,物理盘序列号才是稳定身份)。
func systemVolumeSerial() string {
	root, err := windows.UTF16PtrFromString(systemDriveLetter() + `\`)
	if err != nil {
		return ""
	}
	var serial uint32
	if err := windows.GetVolumeInformation(root, nil, 0, &serial, nil, nil, nil, 0); err != nil {
		return ""
	}
	return fmt.Sprintf("%08X", serial)
}

// OsBuild 读注册表返回 Windows 版本号(如 "10.0.19045" / "6.1.7601"),供上报的展示字段使用。
func OsBuild() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	major, _, _ := k.GetStringValue("CurrentVersion") // "10.0" / "6.1"
	build, _, errB := k.GetStringValue("CurrentBuildNumber")
	if errB != nil {
		build, _, _ = k.GetStringValue("CurrentBuild") // Win7/8 回退
	}
	if major == "" || build == "" {
		return ""
	}
	return major + "." + build
}
