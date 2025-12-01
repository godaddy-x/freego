package DIC

// YamlConfig 通用配置结构体 - 支持多数据源配置
// 注意：为避免循环依赖，这里重新定义了配置结构体
// 这些结构体与各包中原有结构体字段相同，但为了YAML配置读取的便利性而存在
type YamlConfig struct {
	// RabbitMQ配置 - 支持多数据源，key为数据源名称
	RabbitMQ map[string]*AmqpConfig `yaml:"rabbitmq,omitempty"`

	// MySQL配置 - 支持多数据源，key为数据源名称
	MySQL map[string]*MysqlConfig `yaml:"mysql,omitempty"`

	// MongoDB配置 - 支持多数据源，key为数据源名称
	MongoDB map[string]*MGOConfig `yaml:"mongodb,omitempty"`

	// Redis配置 - 支持多数据源，key为数据源名称
	Redis map[string]*RedisConfig `yaml:"redis,omitempty"`

	// 日志配置 - 支持多日志输出器，key为日志器名称
	Logger map[string]*ZapConfig `yaml:"logger,omitempty"`

	// 应用基本信息
	Server struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Debug   bool   `yaml:"debug"`
		Env     string `yaml:"env"`
	} `yaml:"server,omitempty"`
}

// AmqpConfig RabbitMQ配置 - 与amqp.AmqpConfig字段兼容
type AmqpConfig struct {
	DsName    string `yaml:"ds_name" json:"DsName"`
	Host      string `yaml:"host" json:"Host"`
	Username  string `yaml:"username" json:"Username"`
	Password  string `yaml:"password" json:"Password"`
	Port      int    `yaml:"port" json:"Port"`
	Vhost     string `yaml:"vhost" json:"Vhost"`
	SecretKey string `yaml:"secret_key" json:"SecretKey"`
}

// MysqlConfig MySQL配置 - 与sqld.MysqlConfig字段兼容
type MysqlConfig struct {
	DsName          string `yaml:"ds_name" json:"DsName"`
	Host            string `yaml:"host" json:"Host"`
	Port            int    `yaml:"port" json:"Port"`
	Database        string `yaml:"database" json:"Database"`
	Username        string `yaml:"username" json:"Username"`
	Password        string `yaml:"password" json:"Password"`
	Charset         string `yaml:"charset" json:"Charset"`
	SlowQuery       int64  `yaml:"slow_query" json:"SlowQuery"`
	SlowLogPath     string `yaml:"slow_log_path" json:"SlowLogPath"`
	MaxIdleConns    int    `yaml:"max_idle_conns" json:"MaxIdleConns"`
	MaxOpenConns    int    `yaml:"max_open_conns" json:"MaxOpenConns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"ConnMaxLifetime"`
	ConnMaxIdleTime int    `yaml:"conn_max_idle_time" json:"ConnMaxIdleTime"`
}

// MGOConfig MongoDB配置 - 与sqld.MGOConfig字段兼容
type MGOConfig struct {
	DsName         string   `yaml:"ds_name" json:"DsName"`
	Addrs          []string `yaml:"addrs" json:"Addrs"`
	Direct         bool     `yaml:"direct" json:"Direct"`
	ConnectTimeout int64    `yaml:"connect_timeout" json:"ConnectTimeout"`
	SocketTimeout  int64    `yaml:"socket_timeout" json:"SocketTimeout"`
	Database       string   `yaml:"database" json:"Database"`
	Username       string   `yaml:"username" json:"Username"`
	Password       string   `yaml:"password" json:"Password"`
	PoolLimit      int      `yaml:"pool_limit" json:"PoolLimit"`
	Debug          bool     `yaml:"debug" json:"Debug"`
}

// RedisConfig Redis配置 - 与cache.RedisConfig字段兼容
type RedisConfig struct {
	DsName      string `yaml:"ds_name" json:"DsName"`
	Host        string `yaml:"host" json:"Host"`
	Port        int    `yaml:"port" json:"Port"`
	Password    string `yaml:"password" json:"Password"`
	MaxIdle     int    `yaml:"max_idle" json:"MaxIdle"`
	MaxActive   int    `yaml:"max_active" json:"MaxActive"`
	IdleTimeout int    `yaml:"idle_timeout" json:"IdleTimeout"`
	Network     string `yaml:"network" json:"Network"`
	LockTimeout int    `yaml:"lock_timeout" json:"LockTimeout"`
}

// ZapConfig 日志配置 - 与zlog.ZapConfig字段兼容
type ZapConfig struct {
	Layout     int64       `yaml:"layout"`
	Location   string      `yaml:"location"`
	Level      string      `yaml:"level"`
	Console    bool        `yaml:"console"`
	FileConfig *FileConfig `yaml:"file_config,omitempty"`
}

// FileConfig 日志文件配置 - 与zlog.FileConfig字段兼容
type FileConfig struct {
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
	Compress   bool   `yaml:"compress"`
}

// InitDefaults 初始化默认值，避免nil map
func (c *YamlConfig) InitDefaults() {
	if c.RabbitMQ == nil {
		c.RabbitMQ = make(map[string]*AmqpConfig)
	}
	if c.MySQL == nil {
		c.MySQL = make(map[string]*MysqlConfig)
	}
	if c.MongoDB == nil {
		c.MongoDB = make(map[string]*MGOConfig)
	}
	if c.Redis == nil {
		c.Redis = make(map[string]*RedisConfig)
	}
	if c.Logger == nil {
		c.Logger = make(map[string]*ZapConfig)
	}
}

// GetRabbitMQConfig 获取指定名称的RabbitMQ配置
func (c *YamlConfig) GetRabbitMQConfig(name string) *AmqpConfig {
	if c.RabbitMQ == nil {
		return nil
	}
	return c.RabbitMQ[name]
}

// GetMySQLConfig 获取指定名称的MySQL配置
func (c *YamlConfig) GetMySQLConfig(name string) *MysqlConfig {
	if c.MySQL == nil {
		return nil
	}
	return c.MySQL[name]
}

// GetMongoDBConfig 获取指定名称的MongoDB配置
func (c *YamlConfig) GetMongoDBConfig(name string) *MGOConfig {
	if c.MongoDB == nil {
		return nil
	}
	return c.MongoDB[name]
}

// GetRedisConfig 获取指定名称的Redis配置
func (c *YamlConfig) GetRedisConfig(name string) *RedisConfig {
	if c.Redis == nil {
		return nil
	}
	return c.Redis[name]
}

// GetAllRabbitMQConfigs 获取所有RabbitMQ配置
func (c *YamlConfig) GetAllRabbitMQConfigs() map[string]*AmqpConfig {
	if c.RabbitMQ == nil {
		return make(map[string]*AmqpConfig)
	}
	return c.RabbitMQ
}

// GetAllMySQLConfigs 获取所有MySQL配置
func (c *YamlConfig) GetAllMySQLConfigs() map[string]*MysqlConfig {
	if c.MySQL == nil {
		return make(map[string]*MysqlConfig)
	}
	return c.MySQL
}

// GetAllMongoDBConfigs 获取所有MongoDB配置
func (c *YamlConfig) GetAllMongoDBConfigs() map[string]*MGOConfig {
	if c.MongoDB == nil {
		return make(map[string]*MGOConfig)
	}
	return c.MongoDB
}

// GetAllRedisConfigs 获取所有Redis配置
func (c *YamlConfig) GetAllRedisConfigs() map[string]*RedisConfig {
	if c.Redis == nil {
		return make(map[string]*RedisConfig)
	}
	return c.Redis
}

// GetLoggerConfig 获取指定名称的日志配置
func (c *YamlConfig) GetLoggerConfig(name string) *ZapConfig {
	if c.Logger == nil {
		return nil
	}
	return c.Logger[name]
}

// GetAllLoggerConfigs 获取所有日志配置
func (c *YamlConfig) GetAllLoggerConfigs() map[string]*ZapConfig {
	if c.Logger == nil {
		return make(map[string]*ZapConfig)
	}
	return c.Logger
}
