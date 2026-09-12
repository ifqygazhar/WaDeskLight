//go:build windows

package app

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"unsafe"
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// decodeIconImage accepts PNG or JPEG bytes, the two formats the notification
// bridge can hand over.
func decodeIconImage(data []byte) image.Image {
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		return img
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

// createIconFromImage builds an HICON from a decoded image via a top-down
// 32bpp DIB. Callers own the returned handle and must DestroyIcon it.
func createIconFromImage(img image.Image) uintptr {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width > 256 || height > 256 {
		return 0
	}

	header := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    int32(width),
		Height:   -int32(height), // negative: top-down rows
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	hbmColor, _, _ := procCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&header)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hbmColor == 0 || bits == nil {
		return 0
	}

	pixels := unsafe.Slice((*byte)(bits), width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			i := (y*width + x) * 4
			pixels[i+0] = c.B
			pixels[i+1] = c.G
			pixels[i+2] = c.R
			pixels[i+3] = c.A
		}
	}

	hbmMask, _, _ := procCreateBitmap.Call(uintptr(width), uintptr(height), 1, 1, 0)
	if hbmMask == 0 {
		procDeleteObject.Call(hbmColor)
		return 0
	}

	info := iconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))

	procDeleteObject.Call(hbmColor)
	procDeleteObject.Call(hbmMask)
	return hIcon
}
