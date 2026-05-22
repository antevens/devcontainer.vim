package util

import (
	"io"
	"net"
)

// Main process for port forwarding.
// Relays data communication between the destination and source.
func forward(src net.Conn, dstAddr string) error {
	defer src.Close()

	// Connect to the destination
	dst, err := net.Dial("tcp", dstAddr)
	if err != nil {
		return err
	}
	defer dst.Close()

	// Perform bidirectional data transfer
	// Client -> Destination
	go io.Copy(dst, src)

	// Destination -> Client
	io.Copy(src, dst)

	return nil
}

// Start port forwarding
func StartForwarding(listenAddr, forwardAddr string) error {
	// Create listener
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		// Start listening
		client, err := listener.Accept()
		if err != nil {
			continue
		}

		// Execute the main process (relay)
		go forward(client, forwardAddr)
	}
}
