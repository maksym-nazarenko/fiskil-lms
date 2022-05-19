package app

import (
	"strconv"
	"strings"
	"time"
)

// Configuration holds application configuration
type Configuration struct {
	FlushInterval time.Duration
	FlushSize     int
	DB            struct {
		Address  string
		User     string
		Password string
		Name     string
	}
	Pubsub struct {
		Topic string
	}
}

// BuildConfiguration builds configuration of the app based on env
// todo(maksym): consider using CLI flags if feasible
// todo(maksym): consider using something like github.com/spf13/viper or build own solution
func BuildConfiguration(args []string, envGetter EnvGetterFunc) (*Configuration, error) {
	config := Configuration{
		FlushInterval: 1 * time.Minute,
		FlushSize:     5000,
	}

	flushIntervalStr := envGetter("LMS_FLUSH_INTERVAL")
	if flushIntervalStr != "" {
		flushInterval, err := time.ParseDuration(flushIntervalStr)
		if err != nil {
			return nil, err
		}
		config.FlushInterval = flushInterval
	}

	flushSizeStr := envGetter("LMS_FLUSH_SIZE")
	if flushSizeStr != "" {
		flushSize, err := strconv.Atoi(flushSizeStr)
		if err != nil {
			return nil, err
		}
		config.FlushSize = flushSize
	}

	config.DB.Address = envGetter("LMS_DB_ADDRESS")
	config.DB.Name = envGetter("LMS_DB_NAME")
	config.DB.Password = envGetter("LMS_DB_PASSWORD")
	config.DB.User = envGetter("LMS_DB_USER")

	config.Pubsub.Topic = envGetter("LMS_PUBSUB_TOPIC")
	if strings.TrimSpace(config.Pubsub.Topic) == "" {
		config.Pubsub.Topic = "lms"
	}

	return &config, nil
}
