package main

import (
	"log"

	"github.com/danysz/versionradar/backend/server"
)

func main() {
	app, _, err := server.New(server.Options{})
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
