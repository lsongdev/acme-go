package main

import (
	"context"
	"log"

	"github.com/lsongdev/acme-go/acme"
)

func main() {
	ctx := context.Background()

	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := client.GenerateKey(); err != nil {
		log.Fatal(err)
	}

	_, account, err := client.Register(ctx, &acme.AccountRequest{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	key, err := client.ExportKey()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("account=%s key=%d bytes", account.Status, len(key))
}
