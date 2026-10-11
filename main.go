package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
)

//go:embed ui
var uiAssets embed.FS

func main() {
	// git re-invokes this binary as its todo/message editor during history rewrites
	if ok, err := runEditor(os.Args[1:]); ok {
		if err != nil {
			fatal(err)
		}
		return
	}
	addr := flag.String("addr", "127.0.0.1:8484", "listen address")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	flag.Parse()

	ui, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		fatal(err)
	}
	srv := &server{repos: newRepoStore(), ui: ui}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fatal(err)
	}
	u := "http://" + ln.Addr().String()
	fmt.Println("Gitcord: " + u)
	if !*noOpen {
		go exec.Command("xdg-open", u).Start()
	}
	fatal(http.Serve(ln, srv.routes()))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
