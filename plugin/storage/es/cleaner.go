package es

import "net/http"

type IndexCleaner struct {
	client *http.Client
}

func (*IndexCleaner) Clean() error {

}
