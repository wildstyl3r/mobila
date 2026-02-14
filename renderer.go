package main

import (
	"io"
	"log"

	"github.com/pion/mediadevices/pkg/codec/vpx"
	"github.com/pion/mediadevices/pkg/prop"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func NewVideoRenderer(libp2pStream io.Reader, imgWidget *canvas.Image) {
	dec, _ := vpx.NewDecoder(libp2pStream, prop.Media{
		Video: prop.Video{
			Width:     640,
			Height:    480,
			FrameRate: 25,
		},
	})
	var prevRelease func()
	for {

		if prevRelease != nil {
			prevRelease()
		}
		img, release, err := dec.Read()
		prevRelease = release
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("Ошибка декодирования:", err)
			continue
		}
		fyne.Do(func() {
			imgWidget.Image = img
			imgWidget.Refresh()
		})
	}
}
