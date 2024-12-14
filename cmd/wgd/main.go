package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl"
	"os"
)

func main() {
	fmt.Println("Hello, World!")

	logger := createLogger()

	client, err := wgctrl.New()

	if err != nil {
		logger.WithError(err).Fatal("failed to open client")
	}

	devices, err := client.Devices()

	if err != nil {
		logger.WithError(err).Fatal("failed to get devices")
	}

	for _, device := range devices {
		logger.WithField("device", device).Info("device")
	}

	logger.Info("done")
}

func createLogger() *log.Entry {
	logger := log.New()
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)

	return log.NewEntry(logger)
}
