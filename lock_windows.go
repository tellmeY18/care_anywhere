package main

import "errors"

func lockState(string) (func(), error) {
	return nil, errors.New("Windows lifecycle is not yet supported")
}
