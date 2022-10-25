// Package main contains main app entry point
package main

import (
	"log"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/pomerium/ingress-controller/cmd"
	_ "github.com/pomerium/ingress-controller/internal"
)

func main() {
	c, err := cmd.RootCommand()
	if err != nil {
		log.Fatal(err)
	}

	if err = c.Execute(); err != nil {
		log.Fatal(err)
	}
}
