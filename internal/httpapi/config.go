package httpapi

type Config struct {
	// The port of the HTTPs server
	HttpPort uint16 `envconfig:"HTTPS_PORT" default:"8080"`

	// The port of the HTTPs server
	ProbesPorts uint16 `envconfig:"PROBES_PORTS" default:"8081"`

	// Kubeconfig specifies path to a kubeconfig file if the server is run outside of a cluster
	Kubeconfig string `envconfig:"KUBECONFIG" default:""`

	// Verbosity of the logger.
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}
