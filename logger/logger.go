package logger

import (
	"log"
	"os"
)

type AppLogger struct {
	Info  *log.Logger
	Error *log.Logger
}

func NewAppLogger() *AppLogger {
	if err := os.MkdirAll("logs", os.ModePerm); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	infoFile, err := os.OpenFile("logs/info.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open info log file: %v", err)
	}

	errorFile, err := os.OpenFile("logs/error.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open error log file: %v", err)
	}

	return &AppLogger{
		Info:  log.New(infoFile, "INFO: ", log.Ldate|log.Ltime),
		Error: log.New(errorFile, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
	}
}
