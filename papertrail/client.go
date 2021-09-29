package papertrail

import (
	"encoding/json"
	"fmt"
	"log/syslog"
	"regexp"
	"strings"

	"github.com/golang-collections/go-datastructures/queue"
	cfg "github.com/manna-delivery/papertrail-lambda-extension/config"
	"github.com/manna-delivery/papertrail-lambda-extension/logsapi"
	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{"agent": "logsApiAgent"})

// PTLogger is the logger that writes the logs received from Logs API to S3
type PTLogger struct {
	host     string
	protocol string
	logLevel string
	dialer   *syslog.Writer
	logQueue *queue.Queue
	logRegex string
}

// LambdaLogRecord is log record send from Lambda function
type LambdaLogRecord struct {
	Time       string            `json:"time"`
	Record     LambdaRecordField `json:"record"`
	RecordType string            `json:"type"`
}

type LambdaRecordField struct {
	record string
}

func (r *LambdaRecordField) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.record)
}

func (r *LambdaRecordField) UnmarshalJSON(data []byte) error {
	var raw interface{}
	json.Unmarshal(data, &raw)
	switch raw := raw.(type) {
	case string:
		*r = LambdaRecordField{raw}
	case map[string]interface{}:
		jr, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		*r = LambdaRecordField{string(jr)}
	}
	return nil
}

// PapertrailLogRecord is log record sent to Papertrail
type PapertrailLogRecord struct {
	Time              string `json:"time"`
	Log               string `json:"log"`
	Level             string `json:"level"`
	LambdaTime        string `json:"lambda_time"`
	LambdaType        string `json:"lambda_type"`
	LambdaExecutionId string `json:"lambda_exec_id"`
}

// getSyslogLevel return syslog priority (combination of severity and facility)
func getSyslogLevel(level string) syslog.Priority {
	priority := syslog.LOG_INFO
	switch strings.ToLower(level) {
	case "debug":
		priority = syslog.LOG_DEBUG
	case "info":
		priority = syslog.LOG_INFO
	case "warn":
		priority = syslog.LOG_WARNING
	case "warning":
		priority = syslog.LOG_WARNING
	case "error":
		priority = syslog.LOG_ERR
	case "fatal":
		priority = syslog.LOG_CRIT
	}
	return priority | syslog.LOG_KERN
}

// NewPTClient returns a Papertrail Client
func NewPTClient(config *cfg.LambdaExtensionConfig, logQueue *queue.Queue) (*PTLogger, error) {
	dialer, err := syslog.Dial(config.PapertrailProtocol, config.PapertrailAddress, getSyslogLevel(config.PapertrailLogLevel), strings.ToLower(config.FunctionName))
	if err != nil {
		return nil, fmt.Errorf("failed connecting to %s://%s: %v", config.PapertrailProtocol, config.PapertrailAddress, err)
	}
	return &PTLogger{
		host:     config.PapertrailAddress,
		protocol: config.PapertrailProtocol,
		logLevel: config.PapertrailLogLevel,
		dialer:   dialer,
		logQueue: logQueue,
		logRegex: config.LogRegex,
	}, nil
}

func (d *PTLogger) FlushLogQueue() {
	var logsStr string = ""
	logger.Debugf("Queue size: %d", d.logQueue.Len())
	for !(d.logQueue.Empty()) || (strings.Contains(logsStr, string(logsapi.RuntimeDone))) {
		foo := d.logQueue.Empty()
		logger.Info(foo)
		var records []LambdaLogRecord
		logs, err := d.logQueue.Get(1)
		if err != nil {
			logger.Error(err)
			return
		}
		logsStr = fmt.Sprintf("%v", logs[0])
		logger.Debugf("Message: %s", logsStr)
		err = json.Unmarshal([]byte(logsStr), &records)
		if err != nil {
			logger.Error(err)
			return
		}
		for _, record := range records {
			err = d.ParseAndSend(record)
			if err != nil {
				logger.Error(err)
			}
		}
	}
}
func (d *PTLogger) splitLogByRegex(log string) map[string]string {
	var compRegEx = regexp.MustCompile(d.logRegex)
	match := compRegEx.FindStringSubmatch(log)

	logMap := make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			logMap[name] = match[i]
		}
	}
	return logMap
}

func (d *PTLogger) ParseAndSend(r LambdaLogRecord) error {
	var err error
	var ptRecord PapertrailLogRecord

	switch {
	case r.RecordType == "function":
		splitRecord := d.splitLogByRegex(r.Record.record)
		ptRecord = PapertrailLogRecord{
			Time:              splitRecord["Date"],
			Log:               splitRecord["Message"],
			Level:             splitRecord["Level"],
			LambdaTime:        r.Time,
			LambdaType:        r.RecordType,
			LambdaExecutionId: splitRecord["UUID"],
		}
	case strings.HasPrefix(r.RecordType, "platform"):
		var record map[string]string
		json.Unmarshal([]byte(r.Record.record), &record)
		ptRecord = PapertrailLogRecord{
			Time:              r.Time,
			Log:               r.Record.record,
			Level:             "DEBUG",
			LambdaTime:        r.Time,
			LambdaType:        r.RecordType,
			LambdaExecutionId: record["requestId"],
		}
	}

	log, err := json.Marshal(ptRecord)
	if err != nil {
		return err
	}
	switch strings.ToLower(ptRecord.Level) {
	case "debug":
		err = d.Debug(string(log))
	case "info":
		err = d.Info(string(log))
	case "warn":
		err = d.Warn(string(log))
	case "warning":
		err = d.Warn(string(log))
	case "error":
		err = d.Error(string(log))
	case "fatal":
		err = d.Fatal(string(log))
	default:
		err = d.Info(string(log))
	}

	return err
}

func (d *PTLogger) Debug(log string) error {
	err := d.dialer.Debug(log)
	return err
}
func (d *PTLogger) Info(log string) error {
	err := d.dialer.Info(log)
	return err
}
func (d *PTLogger) Warn(log string) error {
	err := d.dialer.Warning(log)
	return err
}
func (d *PTLogger) Error(log string) error {
	err := d.dialer.Err(log)
	return err
}
func (d *PTLogger) Fatal(log string) error {
	err := d.dialer.Crit(log)
	return err
}
func (d *PTLogger) Shutdown() error {
	logger.Debug("Closing Papertrail connection on shutdown")
	err := d.dialer.Close()
	return err
}
