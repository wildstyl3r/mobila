package main

import (
	"context"
	"io"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/pion/mediadevices/pkg/prop"
)

func startVideoSource(ctx context.Context, libp2pStream io.Writer) {
	vpxParams, _ := vpx.NewVP8Params()
	vpxParams.BitRate = 500_000
	vpxParams.KeyFrameInterval = 10

	s := mediadevices.NewCodecSelector(mediadevices.WithVideoEncoders(&vpxParams))

	stream, _ := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
			c.FrameRate = prop.Float(25)
		},
		Codec: s,
	})

	vt := stream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	encodedReader, _ := vt.NewEncodedReader("vp8")

	for {
		frame, _, err := encodedReader.Read()
		if err != nil {
			return
		}

		libp2pStream.Write(frame.Data)
	}
}
