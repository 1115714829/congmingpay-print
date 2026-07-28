package preview

import (
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

var (
	sysFontOnce sync.Once
	sysFontData []byte
)

func loadFace(px float64) (font.Face, error) {
	sysFontOnce.Do(func() {
		for _, p := range []string{
			`C:\Windows\Fonts\simhei.ttf`,
			`C:\Windows\Fonts\simkai.ttf`,
			`C:\Windows\Fonts\simsun.ttc`,
			`C:\Windows\Fonts\msyh.ttc`,
		} {
			if b, err := os.ReadFile(p); err == nil {
				sysFontData = b
				return
			}
		}
	})
	if px < 10 {
		px = 10
	}
	if px > 28 {
		px = 28
	}
	if len(sysFontData) > 0 {
		if f, err := faceFromBytes(sysFontData, px); err == nil {
			return f, nil
		}
	}
	return basicfont.Face7x13, nil
}

func faceFromBytes(b []byte, px float64) (font.Face, error) {
	if col, err := opentype.ParseCollection(b); err == nil && col.NumFonts() > 0 {
		ft, err := col.Font(0)
		if err != nil {
			return nil, err
		}
		return opentype.NewFace(ft, &opentype.FaceOptions{Size: px, DPI: 72})
	}
	ft, err := opentype.Parse(b)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ft, &opentype.FaceOptions{Size: px, DPI: 72})
}
