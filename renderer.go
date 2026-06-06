package main

import (
	"log"
	"os"
	"sync"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

var (
	rendererOnce sync.Once
	mdRenderer   *glamour.TermRenderer
)

func terminalWidth() int {
	const fallbackWidth = 100
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return fallbackWidth
	}

	w, _, err := term.GetSize(fd)
	if err != nil {
		return fallbackWidth
	}

	if w < 50 {
		return 50
	}
	return w - 4
}

func getRenderer() *glamour.TermRenderer {
	rendererOnce.Do(func() {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(terminalWidth()),
		)
		if err != nil {
			log.Fatal(err)
		}
		mdRenderer = r
	})

	return mdRenderer
}

func renderToString(text string) string {
	out, err := getRenderer().Render(text)
	if err != nil {
		log.Fatal(err)
	}
	return out
}

func render(text string) {
	uiPrint(renderToString(text))
}
