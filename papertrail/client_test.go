package papertrail

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"testing"

	"github.com/golang-collections/go-datastructures/queue"
	cfg "github.com/manna-delivery/papertrail-lambda-extension/config"
	"github.com/stretchr/testify/assert"
)

func setupEnv(t *testing.T) {
	t.Helper()
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "pt_log_test")
	os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "Latest$")
	os.Setenv("AWS_LAMBDA_LOG_GROUP_NAME", "/aws/lambda/manna_log_test")
	os.Setenv("AWS_LAMBDA_LOG_STREAM_NAME", "2021/09/11/[$LATEST]8809a25404f146f09ab0d931891d238a")

}

type testLogRecord = struct {
	Time       string
	Record     json.RawMessage
	RecordType string
	TestRegex  string
}

var testNodeJSMessages = []testLogRecord{
	{
		RecordType: "platform.start",
		Time:       "2021-09-18T21:13:42.195Z",
		Record:     json.RawMessage(`{"requestId": "e2eb082d-e007-4a61-b5ef-203b96e51c01", "version": "$LATEST"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.195Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\",\\"version\\":\\"\$LATEST\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.195Z","lambda_type":"platform\.start","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "platform.logsSubscription",
		Time:       "2021-09-18T21:13:42.337Z",
		Record:     json.RawMessage(`{"name":"papertrail-lambda-extension","state":"Subscribed","types":["platform","function"]}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.337Z","log":"{\\"name\\":\\"papertrail-lambda-extension\\",\\"state\\":\\"Subscribed\\",\\"types\\":\[\\"platform\\",\\"function\\"\]}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.337Z","lambda_type":"platform\.logsSubscription","lambda_exec_id":""}\n$`,
	},
	{
		RecordType: "platform.extension",
		Time:       "2021-09-18T21:13:42.337Z",
		Record:     json.RawMessage(`{"events":["INVOKE","SHUTDOWN"],"name":"papertrail-lambda-extension","state":"Ready"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.337Z","log":"{\\"events\\":\[\\"INVOKE\\",\\"SHUTDOWN\\"\],\\"name\\":\\"papertrail-lambda-extension\\",\\"state\\":\\"Ready\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.337Z","lambda_type":"platform\.extension","lambda_exec_id":""}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	INFO	info message`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"info message","level":"INFO","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	ERROR	error message`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"error message","level":"ERROR","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	// Skipped due issue sending messages not in bulk, TODO send messages in bulk and check in bulk
	// {
	// 	RecordType: "platform.runtimeDone",
	// 	Time:       "2021-09-18T21:13:42.351Z",
	// 	Record:     json.RawMessage(`{"requestId": "e2eb082d-e007-4a61-b5ef-203b96e51c01", "status": "success"}`),
	// 	TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.351Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\",\\"status\\":\\"\success\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.351Z","lambda_type":"platform\.runtimeDone","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	// },
	{
		RecordType: "platform.end",
		Time:       "2021-09-18T21:13:43.201Z",
		Record:     json.RawMessage(`{"requestId":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:43\.201Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:43\.201Z","lambda_type":"platform\.end","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "platform.report",
		Time:       "2021-09-18T21:13:43.201Z",
		Record:     json.RawMessage(`{"metrics":{"billedDurationMs":861,"durationMs":860.9,"initDurationMs":203.81,"maxMemoryUsedMB":61,"memorySizeMB":128},"requestId":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:43\.201Z","log":"{\\"metrics\\":{\\"billedDurationMs\\":861,\\"durationMs\\":860.9,\\"initDurationMs\\":203.81,\\"maxMemoryUsedMB\\":61,\\"memorySizeMB\\":128},\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:43\.201Z","lambda_type":"platform\.report","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
}
var testPythonMessages = []testLogRecord{
	{
		RecordType: "platform.start",
		Time:       "2021-09-18T21:13:42.195Z",
		Record:     json.RawMessage(`{"requestId": "e2eb082d-e007-4a61-b5ef-203b96e51c01", "version": "$LATEST"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.195Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\",\\"version\\":\\"\$LATEST\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.195Z","lambda_type":"platform\.start","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "platform.logsSubscription",
		Time:       "2021-09-18T21:13:42.337Z",
		Record:     json.RawMessage(`{"name":"papertrail-lambda-extension","state":"Subscribed","types":["platform","function"]}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.337Z","log":"{\\"name\\":\\"papertrail-lambda-extension\\",\\"state\\":\\"Subscribed\\",\\"types\\":\[\\"platform\\",\\"function\\"\]}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.337Z","lambda_type":"platform\.logsSubscription","lambda_exec_id":""}\n$`,
	},
	{
		RecordType: "platform.extension",
		Time:       "2021-09-18T21:13:42.337Z",
		Record:     json.RawMessage(`{"events":["INVOKE","SHUTDOWN"],"name":"papertrail-lambda-extension","state":"Ready"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.337Z","log":"{\\"events\\":\[\\"INVOKE\\",\\"SHUTDOWN\\"\],\\"name\\":\\"papertrail-lambda-extension\\",\\"state\\":\\"Ready\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.337Z","lambda_type":"platform\.extension","lambda_exec_id":""}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`[DEBUG]	2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	debug message: {"foo": "bar"}`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"debug message: {\\"foo\\": \\"bar\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`[INFO]	2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	info message`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"info message","level":"INFO","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`[WARNING]	2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	warning message`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"warning message","level":"WARNING","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "function",
		Time:       "2021-09-18T21:13:42.339Z",
		Record: json.RawMessage(fmt.Sprintf(`"%s"`, jsonEscape(`[ERROR]	2021-09-18T21:13:42.339Z	e2eb082d-e007-4a61-b5ef-203b96e51c01	error message`))),
		TestRegex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.339Z","log":"error message","level":"ERROR","lambda_time":"2021-09-18T21:13:42\.339Z","lambda_type":"function","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	// Skipped due issue sending messages not in bulk, TODO send messages in bulk and check in bulk
	// {
	// 	RecordType: "platform.runtimeDone",
	// 	Time:       "2021-09-18T21:13:42.351Z",
	// 	Record:     json.RawMessage(`{"requestId":"e2eb082d-e007-4a61-b5ef-203b96e51c01","status":"success"}`),
	// 	TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:42\.351Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\",\\"status\\":\\"\sucess\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:42\.351Z","lambda_type":"platform\.runtimeDone","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	// },
	{
		RecordType: "platform.end",
		Time:       "2021-09-18T21:13:43.201Z",
		Record:     json.RawMessage(`{"requestId":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:43\.201Z","log":"{\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:43\.201Z","lambda_type":"platform\.end","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
	{
		RecordType: "platform.report",
		Time:       "2021-09-18T21:13:43.201Z",
		Record:     json.RawMessage(`{"metrics":{"billedDurationMs":861,"durationMs":860.9,"initDurationMs":203.81,"maxMemoryUsedMB":61,"memorySizeMB":128},"requestId":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}`),
		TestRegex:  `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?)\S.*\spt_log_test\[\d{1,5}\]:\s\{"time":"2021-09-18T21:13:43\.201Z","log":"{\\"metrics\\":{\\"billedDurationMs\\":861,\\"durationMs\\":860.9,\\"initDurationMs\\":203.81,\\"maxMemoryUsedMB\\":61,\\"memorySizeMB\\":128},\\"requestId\\":\\"e2eb082d-e007-4a61-b5ef-203b96e51c01\\"}","level":"DEBUG","lambda_time":"2021-09-18T21:13:43\.201Z","lambda_type":"platform\.report","lambda_exec_id":"e2eb082d-e007-4a61-b5ef-203b96e51c01"}\n$`,
	},
}

func createServer(t *testing.T) net.Listener {
	t.Helper()
	tcpServer, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Can't create new mock %v", err)
	}
	t.Logf("New %s mock listening on: %s", tcpServer.Addr().Network(), tcpServer.Addr().String())
	return tcpServer
}

func getMessages(t *testing.T, conn net.Conn, messages int) (string, error) {
	t.Helper()
	reader := bufio.NewReader(conn)
	var buffer bytes.Buffer
	for i := 0; i < messages; i++ {
		// Fist 3 chars are size, ignore them
		_, _ = reader.ReadByte()
		_, _ = reader.ReadByte()
		_, _ = reader.ReadByte()
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		buffer.Write([]byte(line))
	}
	return buffer.String(), nil
}

func jsonEscape(i string) string {
	b, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	s := string(b)
	return s[1 : len(s)-1]
}

func TestPapertrailNodeJSLogs(t *testing.T) {
	assert := assert.New(t)
	t.Log("Starting NodeJS logs test")
	setupEnv(t)

	tcpServer := createServer(t)
	os.Setenv("PAPERTRAIL_ADDRESS", tcpServer.Addr().String())
	os.Setenv("PAPERTRAIL_PROTOCOL", tcpServer.Addr().Network())
	defer tcpServer.Close()

	config, err := cfg.FetchConfig()
	assert.Nil(err, "Failed fetching configurations")

	t.Log("Starting new client")
	logQueue := queue.New(config.MaxQueueSize)
	ptClient, err := NewPTClient(config, logQueue)
	assert.Nil(err, "Configured the host address somewhere")
	conn, err := tcpServer.Accept()
	assert.Nil(err, "Failed to open server reader")
	defer conn.Close()

	t.Log("NodeJS logs")
	for i, message := range testNodeJSMessages {
		t.Logf("Sending log: '%s'", message.Record)
		logQueue.Put(fmt.Sprintf(`[{"time":"%s","type":"%s","record":%s}]`, message.Time, message.RecordType, message.Record))
		ptClient.FlushLogQueue()
		response, err := getMessages(t, conn, 1)
		assert.Nil(err, fmt.Sprintf("Failed reading message %d", i))
		assert.Regexp(regexp.MustCompile(message.TestRegex),
			response,
			fmt.Sprintf("Message %d is not as expected", i))
	}
}
func TestPapertrailPythonLogs(t *testing.T) {
	assert := assert.New(t)
	t.Log("Starting Python logs test")
	setupEnv(t)

	tcpServer := createServer(t)
	os.Setenv("PAPERTRAIL_ADDRESS", tcpServer.Addr().String())
	os.Setenv("PAPERTRAIL_PROTOCOL", tcpServer.Addr().Network())
	os.Setenv("LAMBDA_LOG_REGEX", `\[(?P<Level>\b[a-zA-Z]*\b)\]\s*(?P<Date>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d*)?(-\d{2}:\d{2}|Z?))\s*(?P<UUID>\b[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}\b)\s*(?P<Message>.*)`)
	defer tcpServer.Close()

	config, err := cfg.FetchConfig()
	assert.Nil(err, "Failed fetching configurations")

	t.Log("Starting new client")
	logQueue := queue.New(config.MaxQueueSize)
	ptClient, err := NewPTClient(config, logQueue)
	assert.Nil(err, "Configured the host address somewhere")
	conn, err := tcpServer.Accept()
	assert.Nil(err, "Failed to open server reader")
	defer conn.Close()

	for i, message := range testPythonMessages {
		t.Logf("Sending log: '%s'", message.Record)
		logQueue.Put(fmt.Sprintf(`[{"time":"%s","type":"%s","record":%s}]`, message.Time, message.RecordType, message.Record))
		ptClient.FlushLogQueue()
		response, err := getMessages(t, conn, 1)
		assert.Nil(err, fmt.Sprintf("Failed reading message %d", i))
		assert.Regexp(regexp.MustCompile(message.TestRegex),
			response,
			fmt.Sprintf("Message %d is not as expected", i))
	}
}
