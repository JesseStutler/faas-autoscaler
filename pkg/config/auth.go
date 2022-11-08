package config

import (
	"github.com/openfaas/faas-cli/proxy"
	"net/http"
)

//NewBasicAuth is only for testing
func NewBasicAuth(username string, password string) proxy.ClientAuth {
	return &BasicAuthPlainText{
		username: username,
		password: password,
	}
}

type BasicAuthPlainText struct {
	username string
	password string
}

func (auth *BasicAuthPlainText) Set(req *http.Request) error {
	req.SetBasicAuth(auth.username, auth.password)
	return nil
}
