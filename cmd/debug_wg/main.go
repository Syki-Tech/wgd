package main

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

const (
	// Interface flags (you can modify these as needed)
	IFF_UP      = 0x1  // Interface is up
	IFF_RUNNING = 0x40 // Interface is running
	IFREQ_SIZE  = 40   // Size of ifreq structure
)

// struct ifreq is used to configure network interfaces
type ifreq struct {
	Name  [16]byte
	Flags uint16
	_     [2]byte // padding
}

func createNetworkInterface(name string) error {
	// Open a raw socket to interact with the network stack
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return fmt.Errorf("failed to create socket: %v", err)
	}
	defer syscall.Close(fd)

	// Prepare the interface request structure (ifreq) to create the interface
	var req ifreq
	copy(req.Name[:], name)          // Set interface name
	req.Flags = IFF_UP | IFF_RUNNING // Set interface flags to up and running

	// Use ioctl syscall to create the interface (SIOCSIFFLAGS)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(0x8914), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return fmt.Errorf("ioctl failed: %v", errno)
	}

	// Interface created successfully
	fmt.Printf("Successfully created and configured interface %s\n", name)
	return nil
}

func main() {
	// Specify the name of the new interface
	name := "wg15" // WireGuard interface name

	// Create the network interface
	if err := createNetworkInterface(name); err != nil {
		log.Fatalf("Failed to create network interface: %v", err)
	}
}
