package lagerflags

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"code.cloudfoundry.org/lager"
)

const (
	DEBUG = "debug"
	INFO  = "info"
	ERROR = "error"
	FATAL = "fatal"
)

type TimeFormat int

const (
	FormatUnixEpoch TimeFormat = iota
	FormatRFC3339
)

func (t TimeFormat) MarshalJSON() ([]byte, error) {
	if FormatUnixEpoch <= t && t <= FormatRFC3339 {
		return []byte(`"` + t.String() + `"`), nil
	}
	return nil, fmt.Errorf("invalid TimeFormat: %d", t)
}

// Set implements the flag.Getter interface
func (t TimeFormat) Get(s string) interface{} { return t }

// Set implements the flag.Value interface
func (t *TimeFormat) Set(s string) error {
	switch s {
	case "unix-epoch", "0":
		*t = FormatUnixEpoch
	case "rfc3339", "1":
		*t = FormatRFC3339
	default:
		return errors.New(`invalid TimeFormat: "` + s + `"`)
	}
	return nil
}

func (t *TimeFormat) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	// unqote
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}
	return t.Set(string(data))
}

func (t TimeFormat) String() string {
	switch t {
	case FormatUnixEpoch:
		return "unix-epoch"
	case FormatRFC3339:
		return "rfc3339"
	}
	return "invalid"
}

type LagerConfig struct {
	LogLevel      string     `json:"log_level,omitempty"`
	RedactSecrets bool       `json:"redact_secrets,omitempty"`
	TimeFormat    TimeFormat `json:"time_format"`
}

func DefaultLagerConfig() LagerConfig {
	return LagerConfig{
		LogLevel:      string(INFO),
		RedactSecrets: false,
		TimeFormat:    FormatUnixEpoch,
	}
}

var minLogLevel string
var redactSecrets bool
var timeFormat TimeFormat

func AddFlags(flagSet *flag.FlagSet) {
	flagSet.StringVar(
		&minLogLevel,
		"logLevel",
		string(INFO),
		"log level: debug, info, error or fatal",
	)
	flagSet.BoolVar(
		&redactSecrets,
		"redactSecrets",
		false,
		"use a redacting log sink to scrub sensitive values from data being logged",
	)
	flagSet.Var(
		&timeFormat,
		"timeFormat",
		`Format for timestamp in component logs. Valid values are "unix-epoch" and "rfc3339".`,
	)
}

func ConfigFromFlags() LagerConfig {
	return LagerConfig{
		LogLevel:      minLogLevel,
		RedactSecrets: redactSecrets,
		TimeFormat:    timeFormat,
	}
}

func New(component string) (lager.Logger, *lager.ReconfigurableSink) {
	return newLogger(component, minLogLevel, lager.NewWriterSink(os.Stdout, lager.DEBUG))
}

func NewFromSink(component string, sink lager.Sink) (lager.Logger, *lager.ReconfigurableSink) {
	return newLogger(component, minLogLevel, sink)
}

func NewFromConfig(component string, config LagerConfig) (lager.Logger, *lager.ReconfigurableSink) {
	var sink lager.Sink

	if config.TimeFormat == FormatRFC3339 {
		sink = lager.NewPrettySink(os.Stdout, lager.DEBUG)
	} else {
		sink = lager.NewWriterSink(os.Stdout, lager.DEBUG)
	}

	if config.RedactSecrets {
		var err error
		sink, err = lager.NewRedactingSink(sink, nil, nil)
		if err != nil {
			panic(err)
		}

	}

	return newLogger(component, config.LogLevel, sink)
}

func newLogger(component, minLogLevel string, inSink lager.Sink) (lager.Logger, *lager.ReconfigurableSink) {
	var minLagerLogLevel lager.LogLevel

	switch minLogLevel {
	case DEBUG:
		minLagerLogLevel = lager.DEBUG
	case INFO:
		minLagerLogLevel = lager.INFO
	case ERROR:
		minLagerLogLevel = lager.ERROR
	case FATAL:
		minLagerLogLevel = lager.FATAL
	default:
		panic(fmt.Errorf("unknown log level: %s", minLogLevel))
	}

	logger := lager.NewLogger(component)

	sink := lager.NewReconfigurableSink(inSink, minLagerLogLevel)
	logger.RegisterSink(sink)

	return logger, sink
}
