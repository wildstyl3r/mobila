package main

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func MainWindow(app fyne.App, state *State, ctx context.Context) fyne.Window {
	window := app.NewWindow("Mobila")
	window.Resize(fyne.NewSize(800, 600))

	contactAlias := widget.NewEntry()
	contactAlias.SetPlaceHolder("Name")
	contactAddress := widget.NewEntry()
	contactAddress.SetPlaceHolder("Peer ID (12D3KooW...)")
	chatsList := widget.NewList(func() int {
		return len(state.Contacts)
	}, func() fyne.CanvasObject {
		return widget.NewLabel("Placeholder")
	}, func(id widget.ListItemID, canvasObj fyne.CanvasObject) {
		canvasObj.(*widget.Label).SetText(state.Contacts[id].Alias)
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
				}
			}, window)
		addContactForm.Show()
	}), nil, nil, nil, chatsList)

	myIDLabel := widget.NewLabel("")
	myID := container.NewBorder(nil, nil, nil,
		widget.NewButton("Copy", func() {
			app.Clipboard().SetContent(myIDLabel.Text)
		}),
		myIDLabel)

	statusBar := widget.NewLabel("")

	setStatus := func(status string) {
		statusBar.SetText(status)
	}
	setStatus("waiting for password")

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

			fmt.Printf("Node started: %s\n", state.Node.Host.ID())
			myIDLabel.SetText(state.Node.Host.ID().String())
			setStatus("Node started")
			state.LoadContacts()
			chatsList.Refresh()

			startingDialog.Hide()
		})})
	startingDialog.Show()

	window.SetContent(container.NewBorder(myID, statusBar, chatsBorder, nil, layout.NewSpacer()))
	return window
}
