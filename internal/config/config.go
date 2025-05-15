package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Port     int
		Domain   string
		APIToken string `mapstructure:"api_token"`
	} `mapstructure:"server"`
	Database struct {
		Host     string
		Port     int
		Name     string
		User     string
		Password string
		SSLMode  string `mapstructure:"sslmode"`
	} `mapstructure:"database"`
	Security struct {
		JWTSecret      string `mapstructure:"jwt_secret"`
		AdminPassword  string `mapstructure:"admin_password"`
		TokenExpiry    int    `mapstructure:"token_expiry"`
		RandomEmailLen int    `mapstructure:"random_email_length"`
	} `mapstructure:"security"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	
	// Handle environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	
	// Set up environment variables with prefix
	viper.SetEnvPrefix("MAILREADER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("server.port", 8081)
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("security.token_expiry", 24)
	viper.SetDefault("security.random_email_length", 12)

	if err := viper.ReadInConfig(); err != nil {
		// Only error if config file is missing and not overridden by env
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Allow env var override for sensitive fields but keep config file values if present
	// Only override config with environment variables if they are explicitly set
	// This preserves config file values when environment vars aren't present
	if v := viper.GetString("DATABASE_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := viper.GetString("SECURITY_ADMIN_PASSWORD"); v != "" {
		cfg.Security.AdminPassword = v
	}
	if v := viper.GetString("SECURITY_JWT_SECRET"); v != "" {
		cfg.Security.JWTSecret = v
	}
	if v := viper.GetString("SERVER_API_TOKEN"); v != "" {
		cfg.Server.APIToken = v
	}
	
	// If API token is not set after loading config and checking env vars, log a warning
	if cfg.Server.APIToken == "" {
		fmt.Println("Warning: No API token found. Set in config file (server.api_token) or SERVER_API_TOKEN env var.")
	}
	
	// Ensure API token is set - if we have one from the config file, we should use it
	if cfg.Server.APIToken == "" {
		log.Println("Warning: No API token found. Set in config file (server.api_token) or SERVER_API_TOKEN env var.")
	}

	cfg.Server.Domain = strings.TrimSpace(cfg.Server.Domain)

	return &cfg, nil
}
