package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL          string
	HTTPAddress          string
	ShutdownTimeout      time.Duration
	MonitorWorkers       int
	RetentionDays        int
	AdminEmail           string
	AdminPassword        string
	SessionSecure        bool
	SessionIdle          time.Duration
	SessionAbsolute      time.Duration
	TrustProxy           bool
	SecretsEncryptionKey string
}

func Load() (Config, error) {
	return loadWith(os.Getenv, os.ReadFile)
}

func load(getenv func(string) string) (Config, error) {
	return loadWith(getenv, func(string) ([]byte, error) {
		return nil, errors.New("file secrets are unavailable in this test")
	})
}

func loadWith(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	databaseURL, err := secretValue(getenv, readFile, "DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	httpAddress := getenv("HTTP_ADDRESS")
	if httpAddress == "" {
		httpAddress = ":8080"
	}

	shutdownTimeout, err := duration(getenv, "SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	monitorWorkers, err := integer(getenv, "MONITOR_WORKERS", 10, 1, 100)
	if err != nil {
		return Config{}, err
	}

	retentionDays, err := integer(getenv, "RESULT_RETENTION_DAYS", 30, 1, 3650)
	if err != nil {
		return Config{}, err
	}

	sessionSecure, err := boolean(getenv, "SESSION_SECURE_COOKIE", false)
	if err != nil {
		return Config{}, err
	}
	sessionIdle, err := duration(getenv, "SESSION_IDLE_TIMEOUT", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	sessionAbsolute, err := duration(getenv, "SESSION_ABSOLUTE_TIMEOUT", 12*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if sessionAbsolute <= sessionIdle {
		return Config{}, errors.New("SESSION_ABSOLUTE_TIMEOUT must be greater than SESSION_IDLE_TIMEOUT")
	}
	trustProxy, err := boolean(getenv, "TRUST_PROXY_HEADERS", false)
	if err != nil {
		return Config{}, err
	}

	adminPassword, err := secretValue(getenv, readFile, "ADMIN_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	encryptionKey, err := secretValue(getenv, readFile, "SECRETS_ENCRYPTION_KEY")
	if err != nil {
		return Config{}, err
	}

	return Config{
		DatabaseURL:          databaseURL,
		HTTPAddress:          httpAddress,
		ShutdownTimeout:      shutdownTimeout,
		MonitorWorkers:       monitorWorkers,
		RetentionDays:        retentionDays,
		AdminEmail:           getenv("ADMIN_EMAIL"),
		AdminPassword:        adminPassword,
		SessionSecure:        sessionSecure,
		SessionIdle:          sessionIdle,
		SessionAbsolute:      sessionAbsolute,
		TrustProxy:           trustProxy,
		SecretsEncryptionKey: encryptionKey,
	}, nil
}

func secretValue(
	getenv func(string) string,
	readFile func(string) ([]byte, error),
	name string,
) (string, error) {
	value := getenv(name)
	filePath := getenv(name + "_FILE")
	if value != "" && filePath != "" {
		return "", fmt.Errorf("%s and %s_FILE cannot both be set", name, name)
	}
	if filePath == "" {
		return value, nil
	}
	contents, err := readFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r"), nil
}

func boolean(getenv func(string) string, name string, defaultValue bool) (bool, error) {
	value := getenv(name)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func integer(
	getenv func(string) string,
	name string,
	defaultValue, minimum, maximum int,
) (int, error) {
	value := getenv(name)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}

	return parsed, nil
}

func duration(
	getenv func(string) string,
	name string,
	defaultValue time.Duration,
) (time.Duration, error) {
	value := getenv(name)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}

	return parsed, nil
}
