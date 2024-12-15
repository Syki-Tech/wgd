package daemon

import (
	"github.com/sirupsen/logrus"
	"os"
)

func CreateLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)

	return logrus.NewEntry(logger)
}
