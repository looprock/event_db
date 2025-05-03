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

	// Allow env var override for sensitive fields
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

	cfg.Server.Domain = strings.TrimSpace(cfg.Server.Domain)

	return &cfg, nil
}
