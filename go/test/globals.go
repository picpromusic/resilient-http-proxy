package test

import "fmt"

const (
	BackendPort  = 5000
	DataDir      = "/tmp/data"
	CompleteFile = DataDir + "/complete"
	BlockSize    = 10000
	Blocks       = 10
	ProxyPort    = 3000
	CompleteSize = BlockSize * Blocks
)

var BaseURLBackend = fmt.Sprintf("http://127.0.0.1:%d", BackendPort)
var BaseURLProxy = fmt.Sprintf("http://127.0.0.1:%d", ProxyPort)
