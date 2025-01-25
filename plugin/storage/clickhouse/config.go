package clickhouse

import(
	"time"

	"go.opentelemetry.io/collector/config/configtls"
)

const (

)

// Configuration is clickhouse's internal configuration data
type Configuration struct{
	Connection Connection `mapstructure:"connection"`

}

type Connection struct {
	// Servers contains a list of hosts that are used to connect to the cluster.
	Servers []string `mapstructure:"servers" valid:"required,url"`
	// LocalDC contains the name of the local Data Center (DC) for DC-aware host selection
	LocalDC string `mapstructure:"local_dc"`
	// Database is the database name for Jaeger service on the server
	Database string `mapstructure:"database_name"`
	// The port used when dialing to a cluster.
	Port int `mapstructure:"port"`
	// DisableAutoDiscovery, if set to true, will disable the cluster's auto-discovery features.
	DisableAutoDiscovery bool `mapstructure:"disable_auto_discovery"`
	// ConnectionsPerHost contains the maximum number of open connections for each host on the cluster.
	ConnectionsPerHost int `mapstructure:"connections_per_host"`
	// ReconnectInterval contains the regular interval after which the driver tries to connect to
	// nodes that are down.
	ReconnectInterval time.Duration `mapstructure:"reconnect_interval"`
	// SocketKeepAlive contains the keep alive period for the default dialer to the cluster.
	SocketKeepAlive time.Duration `mapstructure:"socket_keep_alive"`
	// TLS contains the TLS configuration for the connection to the cluster.
	TLS configtls.ClientConfig `mapstructure:"tls"`
	// Timeout contains the maximum time spent to connect to a cluster.
	DialTimeout time.Duration `mapstructure:"timeout"`
	// MaxIdleConns is the number of connections the pool will keep idle. Default value is 5
	MaxIdleConns int
	// MaxOpenConns is the maximum number of active connections to the database at any time.
	// Default value is MaxIdleConns + 5
	MaxOpenConns int
	// ConnMaxLifetime is the maximum lifetime of a connection in the pool. Default value is 1 hour
	// After that connection is stopped and new connection is made
	ConnMaxLifetime time.Duration
	//ConnOpenStrategy determines how the list of node addresses should be consumed and used
	// to open connections. Refer to: https://clickhouse.com/docs/en/integrations/go#connection-settings
	ConnOpenStrategy ConnOpenStrategy
	// Authenticator contains the details of the authentication mechanism that is used for
	// connecting to a cluster.
	Authenticator Authenticator `mapstructure:"auth"`
	// ProtoVersion contains the version of the native protocol to use when connecting to a cluster.
	ProtoVersion int `mapstructure:"proto_version"`
	// Compression method used by server
	// Takes only 2 values: LZ4 or ZSTD
	Compression string `mapstructure:"compression"`
}

// Authenticator holds the authentication properties needed to connect to a Clickhouse cluster.
type Authenticator struct {
	Basic BasicAuthenticator `mapstructure:"basic"`
}

// BasicAuthenticator holds the username and password for a password authenticator for a Clickhouse cluster.
type BasicAuthenticator struct {
	Username              string   `mapstructure:"username"`
	Password              string   `mapstructure:"password"`
}

type ConnOpenStrategy uint8


func DefaultConfig () *Configuration {

	return &Configuration{

	}
}