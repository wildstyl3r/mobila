package main

import (
	"fmt"
	"image"
	"io"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/prop"
)

type VideoWidget struct {
	widget.BaseWidget
	raster  *canvas.Image
	label   *widget.Label
	id      int
	mu      sync.Mutex
	content *fyne.Container

	// Callback для обработки нажатия
	OnTapped func(id int)
}

func NewVideoWidget(id int, onTapped func(int), caption string) *VideoWidget {
	vw := &VideoWidget{
		id:       id,
		raster:   canvas.NewImageFromImage(nil),
		label:    widget.NewLabel(caption),
		OnTapped: onTapped,
	}
	vw.raster.FillMode = canvas.ImageFillContain
	vw.label.Alignment = fyne.TextAlignCenter
	vw.content = container.NewBorder(nil, vw.label, nil, nil, vw.raster)
	vw.ExtendBaseWidget(vw)
	return vw
}
func (vw *VideoWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(vw.content)
}

func (vw *VideoWidget) Tapped(_ *fyne.PointEvent) { vw.OnTapped(vw.id) }

func (vw *VideoWidget) SetCaption(caption string) { vw.label.SetText(caption) }

func (vw *VideoWidget) UpdateFrame(img image.Image) {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	vw.raster.Image = img
	vw.raster.Refresh()
}

func CreateVideoPad(peers []string) (fyne.CanvasObject, map[string]*VideoWidget) {
	selectedID := -1
	mainContainer := container.NewStack()

	videos := make(map[string]*VideoWidget, len(peers))
	rows := int(math.Ceil(math.Sqrt(float64(len(peers)))))
	refreshLayout := func() {
		if selectedID == -1 {
			grid := container.NewAdaptiveGrid(rows)
			for _, video := range videos {
				video.raster.Resize(fyne.NewSize(100, 100))
				grid.Add(video)
			}
			mainContainer.Objects = []fyne.CanvasObject{grid}
		} else {
			scrollList := container.NewHBox()
			for i, peerID := range peers {
				if i == selectedID {
					continue
				}
				vid := videos[peerID]
				vid.raster.SetMinSize(fyne.NewSize(150, 150))
				scrollList.Add(vid)
			}

			bigVid := videos[peers[selectedID]]
			bigVid.raster.SetMinSize(fyne.NewSize(0, 0))

			bottomScroll := container.NewHScroll(scrollList)
			bottomScroll.SetMinSize(fyne.NewSize(0, 150))

			focusedLayout := container.NewBorder(nil, bottomScroll, nil, nil, bigVid)
			mainContainer.Objects = []fyne.CanvasObject{focusedLayout}
		}
	}
	handleTap := func(i int) {
		if i == selectedID {
			selectedID = -1
		} else {
			selectedID = i
		}
		refreshLayout()
	}
	for i, peerID := range peers {
		videos[peerID] = NewVideoWidget(i, handleTap, fmt.Sprintf("%d", i))
	}
	refreshLayout()
	return mainContainer, videos
}

func GetCameraTracks() (videoTrack *mediadevices.VideoTrack, audioTrack *mediadevices.AudioTrack) {
	vpxParams, _ := vpx.NewVP8Params()
	vpxParams.BitRate = 500_000
	opusParams, _ := opus.NewParams()
	opusParams.BitRate = 48_000

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&vpxParams),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
			c.FrameRate = prop.Float(30)
		},
		Audio: func(mtc *mediadevices.MediaTrackConstraints) {
			mtc.SampleRate = prop.Int(48000)
			mtc.SampleSize = prop.IntExact(2)
		},
		Codec: codecSelector,
	})
	if err != nil {
		fmt.Printf("error getting user media: %v\n", err)
		return
	}
	return stream.GetVideoTracks()[0].(*mediadevices.VideoTrack), stream.GetAudioTracks()[0].(*mediadevices.AudioTrack)
}

func CreateEncoder(pw *io.PipeWriter) (videoConsumer, audioConsumer webm.BlockWriteCloser) {
	fmt.Println("create encoder")

	fmt.Println("create encoder: block writer")
	ws, _ := webm.NewSimpleBlockWriter(pw, []webm.TrackEntry{
		{Name: "Video", TrackNumber: 1, CodecID: "V_VP8", TrackType: 1,
			Video: &webm.Video{PixelWidth: 640, PixelHeight: 480}},
		{Name: "Audio", TrackNumber: 2, CodecID: "A_OPUS", TrackType: 2,
			Audio: &webm.Audio{SamplingFrequency: 48000.0, Channels: 2}},
	})
	return ws[0], ws[1]
}
