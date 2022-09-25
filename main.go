package main

import (
	"context"

	"golang.design/x/clipboard"
)

func main() {
	// clipboard.Read(clipboard.FmtText)
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}
	// changed := clipboard.Write(clipboard.FmtText, []byte("Hello, World!1"))
	// <-changed
	changed := clipboard.Watch(context.Background(), clipboard.FmtText)
	for data := range changed {
		println(string(data))
	}
	// println(`"text data" is no longer available from clipboard.`)
}
