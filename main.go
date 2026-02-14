// package main

// import (
// 	"context"
// 	"fmt"
// 	"sync"
// 	"time"

// 	"fyne.io/fyne/v2"
// 	"fyne.io/fyne/v2/app"
// 	"fyne.io/fyne/v2/container"
// 	"fyne.io/fyne/v2/widget"

// 	"github.com/libp2p/go-libp2p"
// 	dht "github.com/libp2p/go-libp2p-kad-dht"
// 	"github.com/libp2p/go-libp2p/core/peer"
// 	"github.com/multiformats/go-multiaddr"
// 	// "://github.com/core/host"
// 	// "://github.com/core/peer"
// 	// "://github.com"
// )

// func main() {
// 	myApp := app.New()
// 	myWindow := myApp.NewWindow("libp2p Explorer")

// 	ctx := context.Background()
// 	path, err := getIdentityPath()
// 	if err != nil {
// 		panic(err)
// 	}

// 	privKey, err := loadOrGenerateIdentity(path)
// 	if err != nil {
// 		panic(err)
// 	}
// 	h, _ := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),libp2p.Identity(privKey))
// 	kDHT, _ := dht.New(ctx, h)

// 	// --- Данные для UI ---
// 	var peerList []peer.ID
// 	var mu sync.Mutex

// 	// --- Виджеты ---
// 	peerListWidget := widget.NewList(
// 		func() int { return len(peerList) },
// 		func() fyne.CanvasObject { return widget.NewLabel("Peer ID") },
// 		func(i widget.ListItemID, o fyne.CanvasObject) {
// 			mu.Lock()
// 			defer mu.Unlock()
// 			o.(*widget.Label).SetText(peerList[i].String())
// 		},
// 	)

// 	addrInput := widget.NewEntry()
// 	addrInput.SetPlaceHolder("Enter Multiaddress (e.g. /ip4/1.2.3.4/tcp/1234/p2p/...)")

// 	connectBtn := widget.NewButton("Connect Manually", func() {
// 		ma, err := multiaddr.NewMultiaddr(addrInput.Text)
// 		if err != nil {
// 			fmt.Println("Invalid addr:", err)
// 			return
// 		}
// 		info, _ := peer.AddrInfoFromP2pAddr(ma)
// 		h.Connect(ctx, *info)
// 		addrInput.SetText("")
// 	})

// 	// --- Логика обновления ---
// 	go func() {
// 		kDHT.Bootstrap(ctx)
// 		for _, addr := range dht.DefaultBootstrapPeers {
// 			pi, _ := peer.AddrInfoFromP2pAddr(addr)
// 			go h.Connect(ctx, *pi)
// 		}

// 		for {
// 			time.Sleep(2 * time.Second)
// 			mu.Lock()
// 			peerList = h.Network().Peers()
// 			mu.Unlock()
// 			fyne.Do(func() {
// 				peerListWidget.Refresh()
// 			})
// 		}
// 	}()

// 	// --- Компоновка ---
// 	topContent := container.NewVBox(
// 		widget.NewLabelWithStyle("Your ID: "+h.ID().String(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
// 		addrInput,
// 		connectBtn,
// 		widget.NewSeparator(),
// 		widget.NewLabel("Active Peers:"),
// 	)

// 	myWindow.SetContent(container.NewBorder(topContent, nil, nil, nil, peerListWidget))
// 	myWindow.Resize(fyne.NewSize(600, 400))
// 	myWindow.ShowAndRun()
// }

package main

import (
	"context"

	"fyne.io/fyne/v2/app"
)

func main() {
	app := app.New()

	ctx, cancel := context.WithCancel(context.Background())

	state := NewState()

	window := MainWindow(app, state, ctx)

	window.SetOnClosed(func() {
		cancel()
		state.Shutdown()
	})
	window.ShowAndRun()
}
