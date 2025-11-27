package main

import (
	"log"

	"github.com/takeaway1/chatlog-TCOTC/cmd/chatlog"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	chatlog.Execute()
}
