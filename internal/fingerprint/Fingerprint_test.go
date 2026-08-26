package fingerprint

import "testing"

// 固定向量:两端(Windows 与 web 管理端)必须算出同一摘要,改任一端 canonical 规则都会挂。
const fixedVectorHash = "88d57ef5511f4d4bee6d9624364b597fda66c81ebb139f93c898d8a030f7b038"

func TestHashFixedVector(t *testing.T) {
	f := Fingerprint{
		OsType:      "win",
		BoardSerial: "B0ARD-1234",
		CpuID:       "BFEBFBFF000906EA",
		DiskSerials: []string{"WD-WCC6Y9999999"},
	}
	if got := f.Hash(); got != fixedVectorHash {
		t.Fatalf("Hash()=%s, want %s(与 web 端契约不一致)", got, fixedVectorHash)
	}
}

func TestHashIgnoreDisplayFields(t *testing.T) {
	base := Fingerprint{OsType: "win", BoardSerial: "B0ARD-1234", CpuID: "BFEBFBFF000906EA", DiskSerials: []string{"WD-WCC6Y9999999"}}
	withDisplay := base
	withDisplay.OsBuild = "10.0.19045"
	withDisplay.AppVersion = "1.0.0"
	withDisplay.DeviceModel = "DESKTOP"
	if base.Hash() != withDisplay.Hash() {
		t.Fatal("展示字段不应参与摘要")
	}
}

func TestHashTrimAndSortDiskSerials(t *testing.T) {
	a := Fingerprint{OsType: "win", BoardSerial: " X ", DiskSerials: []string{" B ", "A"}}
	b := Fingerprint{OsType: "win", BoardSerial: "X", DiskSerials: []string{"A", "B"}}
	if a.Hash() != b.Hash() {
		t.Fatal("TrimSpace/排序后应等价")
	}
}

func TestValidate(t *testing.T) {
	if err := (Fingerprint{OsType: "win", BoardSerial: "B"}).Validate(); err != nil {
		t.Fatalf("boardSerial 非空应合法: %v", err)
	}
	if err := (Fingerprint{OsType: "win", DiskSerials: []string{"D"}}).Validate(); err != nil {
		t.Fatalf("diskSerials 非空应合法: %v", err)
	}
	if err := (Fingerprint{OsType: "win"}).Validate(); err == nil {
		t.Fatal("全空应 ErrInvalid")
	}
	if err := (Fingerprint{OsType: "win", DiskSerials: []string{" "}}).Validate(); err == nil {
		t.Fatal("空白序列号应视为空")
	}
}
