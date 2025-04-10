package main

func PreviewPaneSetBody(id uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewUid = id
		g_ui.previewPane.SetTitle("Preview")
		g_ui.previewPane.SetText(body, false)
	})
}

func updateStatusBar(text string) {
	if IsOnUiThread() {
		g_ui.statusBar.SetText(text)
	} else {
		g_ui.app.QueueUpdateDraw(func() { g_ui.statusBar.SetText(text) })
	}
}
