package main

// openURL opens a URL with the default browser.
func openURL(url string) {
	if url == "" {
		return
	}
	openWithDefault(url)
}

// openPath opens a file or folder with the default handler.
func openPath(path string) {
	if path == "" {
		return
	}
	openWithDefault(path)
}
