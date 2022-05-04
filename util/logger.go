package util

import (
	"log"
)

// Logger is a simple logger that can be used to log messages to the console

func Log(id uint8, module string, process string, message string) {
	log.Printf("[ %d | %s ] -> %s: %s ", id, module, process, message)
}
