package iface

import (
	"github.com/pkg/errors"
	"net"
	"os/exec"
)

func Create(ifName, address string) error {
	cmd := exec.Command("ip", "link", "add", ifName, "type", "wireguard")

	err := cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to create interface")
	}

	cmd = exec.Command("ip", "address", "add", address, "dev", ifName)

	err = cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to add address")
	}

	cmd = exec.Command("ip", "link", "set", ifName, "up")

	err = cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to bring interface up")
	}

	return nil
}

func Delete(ifName string) error {
	cmd := exec.Command("ip", "link", "delete", ifName)

	err := cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to delete interface")
	}

	return nil
}

func SetRoute(ifName string, ip *net.IPNet) error {
	cmd := exec.Command("ip", "route", "add", ip.String(), "dev", ifName)

	err := cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to add route")
	}

	return nil
}

func FlushRoutes(ifName string) error {
	cmd := exec.Command("ip", "route", "flush", "dev", ifName)

	err := cmd.Run()

	if err != nil {
		return errors.Wrap(err, "failed to flush routes")
	}

	return nil
}
