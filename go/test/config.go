package test

type BackendConfig struct {
	Port               int
	Args               []string
	LogFile            string
	WaitEveryNElements int
	CurrentModified    bool
	RandomEtag         bool
}

type ProxyConfig struct {
	Port     int
	Upstream string
	LogFile  string
	Args     []string
}

type BackendOption func(*BackendConfig)
type ProxyOption func(*ProxyConfig)

// Backend options
func WithBackendPort(port int) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.Port = port
	}
}

func WithBackendArgs(args ...string) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.Args = args
	}
}

func WithBackendWaitEveryNElements(n int) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.WaitEveryNElements = n
	}
}

func WithBackendLogFile(logFile string) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.LogFile = logFile
	}
}

func WithBackendCurrentModified(currentModified bool) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.CurrentModified = currentModified
	}
}

func WithBackendRandomEtag(randomEtag bool) BackendOption {
	return func(cfg *BackendConfig) {
		cfg.RandomEtag = randomEtag
	}
}

// Proxy options
func WithProxyPort(port int) ProxyOption {
	return func(cfg *ProxyConfig) {
		cfg.Port = port
	}
}

func WithProxyUpstream(upstream string) ProxyOption {
	return func(cfg *ProxyConfig) {
		cfg.Upstream = upstream
	}
}

func WithProxyLogFile(logFile string) ProxyOption {
	return func(cfg *ProxyConfig) {
		cfg.LogFile = logFile
	}
}

func WithProxyArgs(args ...string) ProxyOption {
	return func(cfg *ProxyConfig) {
		cfg.Args = args
	}
}
