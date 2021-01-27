package main

import (
	"encoding/base64"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/hashicorp/vault/api"
)

const (
	vaultDir    = `/vault/secrets`
	tlsCertFile = `tls.crt`
	tlsKeyFile  = `tls.key`
)

func main() {
	config := &api.Config{
		Address: os.Getenv("VAULT_ADDRESS"),
	}

	client, err := api.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}
	client.SetToken(os.Getenv("VAULT_TOKEN"))
	c := client.Logical()
	secret, err := c.Read(os.Getenv("VAULT_SECRETS"))
	if err != nil {
		log.Fatal(err)
	}

	m := secret.Data["data"].(map[string]interface{})
	key := m["key"].(string)
	cert := m["cert"].(string)

	decodedKey, err := base64.StdEncoding.DecodeString(key)
	decodedKey = append(decodedKey, '\n')
	if err != nil {
		log.Fatal(err)
	}

	decodedCert, err := base64.StdEncoding.DecodeString(cert)
	decodedCert = append(decodedCert, '\n')
	if err != nil {
		log.Fatal(err)
	}

	tlsKey := path.Join(vaultDir, "tls.key")
	tlsCert := path.Join(vaultDir, "tls.crt")
	err = ioutil.WriteFile(tlsKey, decodedKey, 0666)
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile(tlsCert, decodedCert, 0666)
	if err != nil {
		log.Fatal(err)
	}

	certPath := filepath.Join(vaultDir, tlsCertFile)
	keyPath := filepath.Join(vaultDir, tlsKeyFile)

	mux := http.NewServeMux()
	mux.Handle("/validate", admitFuncHandler(EnforcePodAnnotations))
	server := &http.Server{
		// We listen on port 8443 such that we do not need root privileges or extra capabilities for this server.
		// The Service object will take care of mapping this port to the HTTPS port 443.
		Addr:    ":8443",
		Handler: mux,
	}

	log.Fatal(server.ListenAndServeTLS(certPath, keyPath))
}
