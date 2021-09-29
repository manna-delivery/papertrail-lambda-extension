package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/golang-collections/go-datastructures/queue"
	"github.com/manna-delivery/papertrail-lambda-extension/agent"
	cfg "github.com/manna-delivery/papertrail-lambda-extension/config"
	"github.com/manna-delivery/papertrail-lambda-extension/extension"
	"github.com/manna-delivery/papertrail-lambda-extension/papertrail"
	log "github.com/sirupsen/logrus"
)

var (
	extensionName = path.Base(os.Args[0])
	printPrefix   = fmt.Sprintf("[%s]", extensionName)
	logger        = log.WithFields(log.Fields{"agent": extensionName})
)

func main() {
	// Creating config and performing validation
	var err error
	config, err := cfg.FetchConfig()
	if err != nil {
		logger.Error("Error fetching environment variables: ", err.Error())
	}

	logger.Logger.SetLevel(config.LogLevel)

	extensionClient := extension.NewClient(config.AWSLambdaRuntimeAPI)

	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		cancel()
		logger.Info(printPrefix, "Received", s)
		logger.Info(printPrefix, "Exiting")
	}()

	// Register extension as soon as possible
	_, err = extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	logQueue := queue.New(config.MaxQueueSize)

	// Create Papertrail client
	ptClient, err := papertrail.NewPTClient(config, logQueue)
	if err != nil {
		logger.Fatal(err)
	}
	// Create Logs API agent
	logsApiAgent, err := agent.NewHttpAgent(ptClient, logQueue)
	if err != nil {
		logger.Fatal(err)
	}

	// Subscribe to logs API
	// Logs start being delivered only after the subscription happens.
	agentID := extensionClient.ExtensionID
	err = logsApiAgent.Init(config.AWSLambdaRuntimeAPI, agentID)
	if err != nil {
		logger.Fatal(err)
	}

	// Will block until invoke or shutdown event is received or cancelled via the context.
	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info(printPrefix, " Waiting for event...")
			// This is a blocking call
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				logger.Info(printPrefix, "Error:", err)
				logger.Info(printPrefix, "Exiting")
				return
			}
			// Flush log queue in here after waking up
			// flushLogQueue(false)
			ptClient.FlushLogQueue()
			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				logger.Info(printPrefix, "Received SHUTDOWN event")
				// flushLogQueue(true)
				ptClient.FlushLogQueue()
				logsApiAgent.Shutdown()
				logger.Info(printPrefix, "Exiting")
				return
			}
		}
	}
}
