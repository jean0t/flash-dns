package logger

import (
	"log"
	"os"
)

var (
	logger *log.Logger
)

const (
	Reset       string = "\033[0m"
	Red         string = "\033[31m"
	Green       string = "\033[32m"
	Yellow      string = "\033[33m"
	DefaultPath string = "/var/log/dnsServer.log"
)

func Init(logFile string) error {
	var (
		err error
		f   *os.File
	)
	f, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	logger = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	return nil
}

func Info(msg string) {
	if logger != nil {
		logger.Printf("%s[INFO]%s%s\n", Green, msg, Reset)
	}
}

func Warn(msg string) {
	if logger != nil {
		logger.Printf("%s[Warn]%s%s\n", Yellow, msg, Reset)
	}
}

func Error(msg string) {
	if logger != nil {
		logger.Printf("%s[ERROR]%s%s\n", Red, msg, Reset)
	}
}
