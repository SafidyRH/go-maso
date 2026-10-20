package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL     string
	HTTPAddress     string
	ShutdownTimeout time.Duration
	MonitorWorkers  int
	RetentionDays   int
}

func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	databaseURL := getenv("DATABASE_URL")

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

	return Config{
		DatabaseURL:     databaseURL,
		HTTPAddress:     httpAddress,
		ShutdownTimeout: shutdownTimeout,
		MonitorWorkers:  monitorWorkers,
		RetentionDays:   retentionDays,
	}, nil
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
