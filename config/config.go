package config

import (
	"errors"
	"os"

	"github.com/sirupsen/logrus"
)

// LambdaExtensionConfig config for storing all configurable parameters
type LambdaExtensionConfig struct {
	PapertrailAddress   string
	PapertrailProtocol  string
	PapertrailLogLevel  string
	AWSLambdaRuntimeAPI string
	FunctionName        string
	FunctionVersion     string
	MaxQueueSize        int64
	LambdaRegion        string
	LogLevel            logrus.Level
	LogRegex            string
}

// FetchConfig returning config from env vars or defaults
func FetchConfig() (*LambdaExtensionConfig, error) {

	config := &LambdaExtensionConfig{
		PapertrailAddress:   os.Getenv("PAPERTRAIL_ADDRESS"),
		PapertrailProtocol:  os.Getenv("PAPERTRAIL_PROTOCOL"),
		PapertrailLogLevel:  os.Getenv("PAPERTRAIL_LEVEL"),
		AWSLambdaRuntimeAPI: os.Getenv("AWS_LAMBDA_RUNTIME_API"),
		FunctionName:        os.Getenv("AWS_LAMBDA_FUNCTION_NAME"),
		FunctionVersion:     os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"),
		LambdaRegion:        os.Getenv("AWS_REGION"),
		LogRegex:            os.Getenv("LAMBDA_LOG_REGEX"),
		MaxQueueSize:        5,
	}

	if config.PapertrailAddress == "" {
		return nil, errors.New("Environment variable PAPERTRAIL_ADDRESS is not set.")
	}

	(*config).setDefaults()

	return config, nil
}

func (c *LambdaExtensionConfig) setDefaults() {
	c.LogLevel = logrus.InfoLevel
	if c.PapertrailProtocol == "" {
		c.PapertrailProtocol = "udp"
	}
	if c.PapertrailLogLevel == "" {
		c.PapertrailLogLevel = "INFO"
	}
	if c.LogRegex == "" {
		c.LogRegex = `(?P<Date>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?))\s*(?P<UUID>\b[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}\b)\s*(?P<Level>\b[a-zA-Z]*\b)\s*(?P<Message>.*)`
	}
}
