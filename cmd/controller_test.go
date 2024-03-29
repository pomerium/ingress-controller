package cmd

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlags(t *testing.T) {
	cmd := new(controllerCmd)
	caString := "pvsuDLZHrTr0vDt6+5ghiQ=="
	caData, err := base64.StdEncoding.DecodeString(caString)
	assert.NoError(t, err)
	for k, v := range map[string]string{
		metricsBindAddress:         ":5678",
		healthProbeBindAddress:     ":9876",
		ingressClassControllerName: "class-name",
		annotationPrefix:           "prefix",
		databrokerServiceURL:       "https://host.somewhere.com:8934",
		databrokerTLSCAFile:        "/tmp/tlsca.file",
		databrokerTLSCA:            caString,
		tlsInsecureSkipVerify:      "true",
		tlsOverrideCertificateName: "override",
		namespaces:                 "one,two,three",
		sharedSecret:               "secret",
		debug:                      "true",
		updateStatusFromService:    "some/service",
	} {
		os.Setenv(envName(k), v)
	}
	cmd.setupFlags()
	assert.Equal(t, []string{"one", "two", "three"}, cmd.Namespaces)
	assert.Equal(t, caData, cmd.tlsCA)
	assert.Equal(t, true, cmd.debug)
}
