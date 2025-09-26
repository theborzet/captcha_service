package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Instance InstanceConfig `yaml:"instance"`
	Balancer BalancerConfig `yaml:"balancer"`
	Logging  LoggingConfig  `yaml:"logging"`
	Host     string         // хост сервера капчи (из переменной окружения HOST, не из YAML)
}

type ServerConfig struct {
	MinPort             int `yaml:"min_port"`
	MaxPort             int `yaml:"max_port"`
	MaxShutdownInterval int `yaml:"max_shutdown_interval"`
}

// InstanceConfig - настройки инстанса капчи
type InstanceConfig struct {
	ID            string `yaml:"id"`
	ChallengeType string `yaml:"challenge_type"`
}

// BalancerConfig - настройки подключения к балансеру
type BalancerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

func MustLoad() *Config {
	configPath := fetchConfigPath()
	if configPath == "" {
		panic("config path is empty")
	}

	cfg, err := Load(configPath)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	return cfg
}

// Load загружает конфиг из YAML-файла с переопределением через .env и валидацией
func Load(configPath string) (*Config, error) {
	cfg := &Config{}

	// Загружаем YAML, если файл существует
	if configPath != "" {
		if err := loadYAML(cfg, configPath); err != nil {
			return nil, fmt.Errorf("failed to load YAML: %w", err)
		}
	}

	// Переопределяем из переменных окружения
	overrideFromEnv(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("incorrect config: %w", err)
	}

	return cfg, nil
}

func loadYAML(cfg *Config, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", absPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, cfg)
}

// overrideFromEnv - переопределяет конфиг из переменных окружения(если они заведены) приоритетнее ямла
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("MIN_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1024 && port <= 65535 {
			cfg.Server.MinPort = port
		}
	}
	if v := os.Getenv("MAX_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1024 && port <= 65535 {
			cfg.Server.MaxPort = port
		}
	}
	if v := os.Getenv("MAX_SHUTDOWN_INTERVAL"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Server.MaxShutdownInterval = sec
		}
	}

	if v := os.Getenv("INSTANCE_ID"); v != "" {
		cfg.Instance.ID = v
	}
	if v := os.Getenv("CHALLENGE_TYPE"); v != "" {
		cfg.Instance.ChallengeType = v
	}

	if v := os.Getenv("BALANCER_HOST"); v != "" {
		cfg.Balancer.Host = v
	}
	if v := os.Getenv("BALANCER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port >= 1024 && port <= 65535 {
			cfg.Balancer.Port = port
		}
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = strings.ToLower(v)
	}

	if v := os.Getenv("HOST"); v != "" {
		cfg.Host = v
	}
}

// validate - валидирует конфиг после загрузки
func validate(cfg *Config) error {
	if cfg.Server.MinPort > cfg.Server.MaxPort {
		return fmt.Errorf("min_port (%d) > max_port (%d)", cfg.Server.MinPort, cfg.Server.MaxPort)
	}
	if cfg.Server.MaxShutdownInterval <= 0 {
		return fmt.Errorf("max_shutdown_interval must be > 0")
	}
	if cfg.Instance.ID == "" {
		return fmt.Errorf("instance.id can not be empty")
	}
	if cfg.Balancer.Port < 1024 || cfg.Balancer.Port > 65535 {
		return fmt.Errorf("balancer.port out of range 1024-65535")
	}
	return nil
}

// fetchConfigPath — получает путь к конфигу из флага или env
func fetchConfigPath() string {
	var configPath string

	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.Parse()

	if configPath == "" {
		configPath = os.Getenv("CONFIG_PATH")
	}

	return configPath
}
