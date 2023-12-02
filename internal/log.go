package internal

import (
	"fmt"
	"log"
	"os"
)

type Logging interface {
	Printf(format string, v ...interface{})
}

type logger struct {
	log *log.Logger
}

func (l *logger) Printf(format string, v ...interface{}) {
	_ = l.log.Output(2, fmt.Sprintf(format, v...))
}

var l Logging = &logger{
	log: log.New(os.Stdout, "go-elect: ", log.LstdFlags|log.Lshortfile),
}

func SetLogger(logger Logging) {
	l = logger
}

func GetLogger() Logging {
	return l
}
