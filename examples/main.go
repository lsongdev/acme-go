package main

import (
	"log"

	"github.com/lsongdev/acme-go/acme"
)

func main() {
	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := client.GenerateKey(); err != nil {
		log.Fatal(err)
	}

	key, err := client.ExportKey()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("generated account key (%d bytes); persist it securely before registering", len(key))
}
