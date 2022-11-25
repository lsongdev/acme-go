package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"

	"github.com/lsongdev/acme-go/acme"
)

const accountKey = `
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIGVk46ELEo4K2AdxjrpFdAiPeFOBVSaZtLqYwcIh2HkXoAoGCCqGSM49
AwEHoUQDQgAEp3WfLMfp9Mvhrm8xWex850OrrpVeUgFajsNEn5DwQS3ivH4bEmoJ
VGyorRanOz/Ep3acmt3G3jZfso+gAzZQsw==
-----END EC PRIVATE KEY-----
`

func main() {
	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		AccountURL:   "https://acme-staging-v02.api.letsencrypt.org/acme/acct/133591064",
	})
	if err != nil {
		log.Fatal(err)
	}
	// ====================== KEY BEGIN =======================
	err = client.ImportKey(accountKey)
	if err != nil {
		log.Fatal(err)
	}
	// err = client.GenerateKey()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// key, err := client.ExportKey()
	// log.Println(key)
	// ====================== KEY END ==========================
	// ====================== ACCOUNT BEGIN ====================
	// accountUrl, accResp, err := client.Register(&acme.AccountRequest{
	// 	TermsOfServiceAgreed: true,
	// 	Contact: []string{
	// 		"mailto: song940@gmail.com",
	// 	},
	// })
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Println(accountUrl, accResp)
	// client.AccountURL = accountUrl
	// ====================== ACCOUNT END ==========================
	// ====================== ORDER BEGIN ==========================
	// orderUrl, orderResp, err := client.CreateOrder(&acme.OrderRequest{
	// 	Identifiers: []acme.Identifier{
	// 		{
	// 			Type:  "dns",
	// 			Value: "lsong.org",
	// 		},
	// 	},
	// })
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Println(orderUrl, orderResp)
	orderUrl := "https://acme-staging-v02.api.letsencrypt.org/acme/order/133591064/13901843404"
	order, err := client.GetOrder(orderUrl)
	if err != nil {
		log.Fatal(err)
	}
	if order.Status == acme.OrderStatusPending {
		for _, authUrl := range order.Authorizations {
			auth, err := client.GetAuthorization(authUrl)
			if err != nil {
				log.Fatal(err)
			}
			// log.Println(auth)
			for _, ch := range auth.Challenges {
				// log.Println(ch.ValidationRecord)

				if ch.Type == "dns-01" {
					record, _ := client.DNS01KeyAuthorization(ch.Token)
					log.Println(record.Type, record.Name, record.Content)
					// client.CompleteChallenge(ch.URL)
				}
			}
		}
	}

	if order.Status == acme.OrderStatusReady {
		certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		domainList := []string{
			"lsong.org",
		}
		tpl := &x509.CertificateRequest{
			SignatureAlgorithm: x509.ECDSAWithSHA256,
			PublicKeyAlgorithm: x509.ECDSA,
			PublicKey:          certKey.Public(),
			Subject:            pkix.Name{CommonName: domainList[0]},
			DNSNames:           domainList,
		}
		csrDer, err := x509.CreateCertificateRequest(rand.Reader, tpl, certKey)
		if err != nil {
			log.Fatal(err)
		}
		err = client.FinalizeOrder(order.Finalize, csrDer)
		if err != nil {
			log.Fatal(err)
		}
	}

	if order.Status == acme.OrderStatusValid {
		cert, err := client.GetCertificatePEM(order.Certificate)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(cert)
	}
}
