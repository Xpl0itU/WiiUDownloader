package wiiudownloader

import (
	"io"
	"log"
	"os"
)

type LogLevel int

const (
	Info LogLevel = iota
	Warning
	Error
	Fatal
)

var logLevelStrings = map[LogLevel]string{
	Info:    "[Info]",
	Warning: "[Warning]",
	Error:   "[Error]",
	Fatal:   "[Fatal]",
}

type Logger struct {
	logFile *os.File
	logger  *log.Logger
}

func NewLogger(logFilePath string) (*Logger, error) {
	var logFile *os.File
	var err error

	// If logFilePath is empty, log only to stdout
	if logFilePath == "" {
		return &Logger{
			logger: log.New(os.Stdout, "", log.Ldate|log.Ltime),
		}, nil
	}

	// Open the log file for writing, truncating it if it exists
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		// If unable to open the log file, log the error to stdout
		log.New(os.Stdout, "", log.Ldate|log.Ltime).Printf("[Error] Unable to open log file: %v\n", err)
		return &Logger{
			logger: log.New(os.Stdout, "", log.Ldate|log.Ltime),
		}, nil
	}

	// Create the logger that writes to both stdout and the file
	logger := log.New(io.MultiWriter(os.Stdout, logFile), "", log.Ldate|log.Ltime)

	return &Logger{
		logFile: logFile,
		logger:  logger,
	}, nil
}

func (l *Logger) log(level LogLevel, format string, v ...interface{}) {
	if prefix, ok := logLevelStrings[level]; ok {
		l.logger.Printf(prefix+" "+format, v...)
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.log(Info, format, v...)
}

func (l *Logger) Warning(format string, v ...interface{}) {
	l.log(Warning, format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.log(Error, format, v...)
}

func (l *Logger) Fatal(format string, v ...interface{}) {
	l.log(Fatal, format, v...)
	os.Exit(1)
}
