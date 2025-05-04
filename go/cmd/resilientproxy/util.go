package main

import "log"

func logUpstream(format string, v ...interface{}) {
	log.Printf("\t\t"+format, v...)
}
