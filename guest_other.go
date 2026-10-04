//go:build !linux

package main

import "errors"

func guestMain() error { return errors.New("guest agent runs only inside the Linux appliance") }
