package net

import (
	"log"
	"os/exec"
	"time"

	"github.com/vishvananda/netlink"
)

const (
	pingHost    = "8.8.8.8"
	pingCount   = "1"
	pingTimeout = "3"
)

func executePingCommand() error {
	cmd := exec.Command("ping", "-c", pingCount, "-W", pingTimeout, pingHost)
	return cmd.Run()
}

func HasNetworkConnection() bool {
	return executePingCommand() == nil
}

func HasNetworkConnectionWithTimeout(timeout time.Duration) bool {
	// Create a channel to receive the result
	resultChan := make(chan bool)

	// Run the check in a goroutine
	go func() {
		resultChan <- HasNetworkConnection()
	}()

	// Wait for either the result or timeout
	select {
	case result := <-resultChan:
		return result
	case <-time.After(timeout):
		return false
	}
}

type NetworkListener struct {
	nlHandle *netlink.Handle
	updateCh chan netlink.LinkUpdate
	doneCh   chan struct{}
}

func NewNetworkListener() (*NetworkListener, error) {
	nlHandle, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}

	updateCh := make(chan netlink.LinkUpdate)
	doneCh := make(chan struct{})

	return &NetworkListener{
		nlHandle: nlHandle,
		updateCh: updateCh,
		doneCh:   doneCh,
	}, nil
}

func (nl *NetworkListener) Close() {
	if nl.nlHandle != nil {
		nl.nlHandle.Close()
	}
	close(nl.doneCh)
}

func ListenNetworkChanges(callback func(bool)) {
	listener, err := NewNetworkListener()
	if err != nil {
		log.Printf("Failed to create network listener: %v", err)
		return
	}
	defer listener.Close()

	if err := netlink.LinkSubscribe(listener.updateCh, listener.doneCh); err != nil {
		log.Printf("Failed to subscribe to link updates: %v", err)
		return
	}

	// Monitor network changes
	go func() {
		for {
			select {
			case update := <-listener.updateCh:
				isUp := update.Link.Attrs().OperState == netlink.OperUp
				callback(isUp)
			case <-listener.doneCh:
				return
			}
		}
	}()
}
