package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func showPeersWindow(a fyne.App, h host.Host) {
	win := a.NewWindow("Known Peers")
	var peers []peer.ID = h.Network().Peers()

	list := widget.NewList(
		func() int { return len(peers) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(peers[i].String())
		},
	)

	stop := make(chan struct{})
	win.SetOnClosed(func() {
		close(stop)
	})

	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				peers = h.Network().Peers()
				fyne.Do(func() {
					list.Refresh()
				})
				time.Sleep(time.Second * 3)
			}
		}
	}()

	win.SetContent(container.NewStack(list))
	win.Resize(fyne.NewSize(400, 300))
	win.Show()
}

func MainWindow(app fyne.App, state *State, ctx context.Context) fyne.Window {
	window := app.NewWindow("Mobila")
	window.Resize(fyne.NewSize(800, 600))

	contactAlias := widget.NewEntry()
	contactAlias.SetPlaceHolder("Name")
	contactAddress := widget.NewEntry()
	contactAddress.SetPlaceHolder("Peer ID (12D3KooW...)")
	chatsList := widget.NewList(func() int {
		return len(state.Chats)
	}, func() fyne.CanvasObject {
		return widget.NewLabel("Placeholder")
	}, func(id widget.ListItemID, canvasObj fyne.CanvasObject) {
		canvasObj.(*widget.Label).SetText(state.Chats[id].Name)
	})
	chatsBorder := container.NewBorder(widget.NewButton("Add contact", func() {
		addContactForm := dialog.NewCustomConfirm("Add contact", "Confirm", "Cancel",
			container.NewVBox(
				contactAlias,
				contactAddress,
			), func(confirmed bool) {
				if confirmed {
					state.AddContact(Contact{
						ID:    contactAddress.Text,
						Alias: contactAlias.Text,
					})
					contactAddress.SetText("")
					contactAlias.SetText("")
					state.ReloadContactsAndChats()
					window.Content().Refresh()
				}
			}, window)
		addContactForm.Show()
	}), nil, nil, nil, chatsList)

	myIDLabel := widget.NewLabel("")
	myID := container.NewBorder(nil, nil,
		widget.NewButton("Copy my address", func() {
			app.Clipboard().SetContent(myIDLabel.Text)
		}), nil,
		myIDLabel)

	statusBar := widget.NewLabel("")

	setStatus := func(status string) {
		statusBar.SetText(status)
	}
	setStatus("waiting for password")

	peerBtn := widget.NewButton("Peers: 0", func() {
		showPeersWindow(app, state.Node.Host)
	})

	passEntry := widget.NewPasswordEntry()
	startingDialog := dialog.NewCustomWithoutButtons("Magic word", passEntry, window)

	startingDialog.SetButtons([]fyne.CanvasObject{
		widget.NewButton("Confirm", func() {
			var err error
			state.Store, err = NewStore(passEntry.Text)
			passEntry.SetText("")
			if err != nil {
				fmt.Println("Password error")
				setStatus("Password error")
				return
			}

			state.Node, err = StartNode(ctx, state.Store)
			if err != nil {
				fmt.Println("Node startup error")
				setStatus("Node startup error")
				return
			}

			go func() {
				for {
					count := len(state.Node.Host.Network().Peers())
					fyne.Do(func() {
						peerBtn.SetText(fmt.Sprintf("Peers: %d", count))
					})

					time.Sleep(time.Second * 3)
				}
			}()

			fmt.Printf("Node started: %s\n", state.Node.Host.ID())
			myIDLabel.SetText(state.Node.Host.ID().String())
			setStatus("Node started")
			err = state.Init()
			if err != nil {
				panic(err)
			}
			chatsList.Refresh()

			startingDialog.Hide()
		})})
	startingDialog.Show()

	var selectChat func(id widget.ListItemID)

	summonCallWindow := func() {
		if state.SelectedChat != nil {
			chatsList.OnSelected = nil
			callWindow := app.NewWindow(fmt.Sprintf("Calling %s", state.SelectedChat.Name))
			var switchVideoBtn, switchAudioBtn *widget.Button
			switchVideoBtn = widget.NewButton("Video on", func() {
				state.VideoMutex.Lock()
				state.VideoOn = !state.VideoOn
				if state.VideoOn {
					switchVideoBtn.SetText("Video off")
				} else {
					switchVideoBtn.SetText("Video on")
				}
				state.VideoMutex.Unlock()
			})
			switchAudioBtn = widget.NewButton("Mic on", func() {
				state.AudioMutex.Lock()
				state.AudioOn = !state.AudioOn
				if state.AudioOn {
					switchAudioBtn.SetText("Mic off")
				} else {
					switchAudioBtn.SetText("Mic on")
				}
				state.AudioMutex.Unlock()
			})

			disconnectBtn := widget.NewButton("Disconnect", func() {
				state.LeaveVideoChat()
			})
			callWindow.SetOnClosed(func() {
				state.LeaveVideoChat()
				chatsList.OnSelected = selectChat
			})
			videoPad := state.JoinVideoChat()
			if videoPad != nil {
				callWindow.SetContent(container.NewBorder(nil, container.NewHBox(switchVideoBtn, switchAudioBtn, disconnectBtn), nil, nil, videoPad))
				fmt.Println("set call content")
				callWindow.Resize(fyne.NewSize(600, 600))
				callWindow.Show()
				fmt.Println("show call window")
			} else {
				state.LeaveVideoChat()
				fmt.Println("Unable to create video pad for some reason (selected chat became nil?)")
			}
		}
	}

	chatName := widget.NewLabel("chat placeholder")
	chatTop := container.NewBorder(nil, nil, nil,
		widget.NewButton("Call", summonCallWindow), chatName,
	)

	messageEntry := widget.NewEntry()
	messageSender := func(msg string) {
		fmt.Printf("%s\n", msg)
		messageEntry.SetText("")
	}
	messageEntry.OnSubmitted = messageSender
	messageSend := widget.NewButton(">>", func() {
		messageSender(messageEntry.Text)
	})
	chatSendMessage := container.NewBorder(nil, nil, nil, messageSend, messageEntry)
	messagesList := widget.NewList(func() int {
		if state.SelectedChat != nil {
			return len(state.SelectedChat.Messages)
		} else {
			return 0
		}
	}, func() fyne.CanvasObject {
		text := widget.NewLabel("Message Text")
		text.Alignment = fyne.TextAlignLeading
		time := widget.NewLabel("00:00")
		time.Alignment = fyne.TextAlignTrailing
		return container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				text,
				time,
			),
			layout.NewSpacer(),
		)
	}, func(id widget.ListItemID, o fyne.CanvasObject) {
		line := o.(*fyne.Container)
		leftSpacer := line.Objects[0].(*layout.Spacer)
		container := line.Objects[1].(*fyne.Container)
		rightSpacer := line.Objects[2].(*layout.Spacer)
		textLabel := container.Objects[0].(*widget.Label)
		timeLabel := container.Objects[1].(*widget.Label)
		message := &state.SelectedChat.Messages[id]
		textLabel.SetText(message.Text)
		timeLabel.SetText(message.Sent.Format("HH:MM DD-MM-YY"))
		if message.Author == state.Node.Host.ID().String() {
			rightSpacer.Hide()
		} else {
			leftSpacer.Hide()
		}
	})
	chatStructure := container.NewBorder(chatTop, chatSendMessage, nil, nil, messagesList)
	chatStructure.Hide()
	chatPlaceholder := layout.NewSpacer()
	chatDetails := container.NewStack(chatPlaceholder, chatStructure)
	selectChat = func(id widget.ListItemID) {
		chatStructure.Show()
		chatPlaceholder.Hide()
		chatName.SetText(state.Chats[id].Name)
		state.SelectedChat = &state.Chats[id]
		state.Store.GetFullChat(state.SelectedChat, state.Contacts)
		state.ChatPeersShuffled = append([]string{}, state.SelectedChat.Peers...)
		rand.Shuffle(len(state.ChatPeersShuffled), func(i, j int) {
			state.ChatPeersShuffled[i], state.ChatPeersShuffled[j] = state.ChatPeersShuffled[j], state.ChatPeersShuffled[i]
		})
	}
	chatsList.OnSelected = selectChat

	window.SetContent(container.NewBorder(myID, container.NewBorder(nil, nil, peerBtn, nil, statusBar), chatsBorder, nil, chatDetails))
	return window
}
