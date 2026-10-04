package main

import "errors"

func doctor() error                                    { return errors.New("Windows WHPX runner is not yet supported") }
func bootVM(string, string, manifest) (machine, error) { return nil, doctor() }
