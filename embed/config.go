package embed

import (
	"fmt"
	"net/url"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/microyahoo/etcd-test/pkg/logutil"
)

const (
	DefaultName = "default"

	DefaultLogOutput = "default"
	StdErrLogOutput  = "stderr"
	StdOutLogOutput  = "stdout"

	DefaultListenPeerURLs   = "http://localhost:2380"
	DefaultListenClientURLs = "http://localhost:2379"

	DefaultInitialAdvertisePeerURLs = "http://localhost:2380"
	DefaultAdvertiseClientURLs      = "http://localhost:2379"
)

// Config holds the arguments for configuring an etcd server.
type Config struct {
	Name   string `json:"name"`
	Dir    string `json:"data-dir"`
	WalDir string `json:"wal-dir"`

	LPUrls, LCUrls []url.URL // listen-peer-urls and listen-client-urls
	APUrls, ACUrls []url.URL // initial-advertise-peer-urls and advertise-client-urls

	InitialCluster      string `json:"initial-cluster"`
	InitialClusterToken string `json:"initial-cluster-token"`

	// LogLevel configures log level. Only supports debug, info, warn, error, panic, or fatal. Default 'info'.
	LogLevel string `json:"log-level"`
	// LogOutputs is either:
	//  - "default" as os.Stderr,
	//  - "stderr" as os.Stderr,
	//  - "stdout" as os.Stdout,
	//  - file path to append server logs to.
	// It can be multiple when "Logger" is zap.
	LogOutputs []string `json:"log-outputs"`

	// ZapLoggerBuilder is used to build the zap logger.
	ZapLoggerBuilder func(*Config) error

	// logger logs server-side operations. The default is nil,
	// and "setupLogging" must be called before starting server.
	// Do not set logger directly.
	loggerMu *sync.RWMutex
	logger   *zap.Logger

	// loggerConfig is server logger configuration for Raft logger.
	// Must be either: "loggerConfig != nil" or "loggerCore != nil && loggerWriteSyncer != nil".
	loggerConfig *zap.Config
	// loggerCore is "zapcore.Core" for raft logger.
	// Must be either: "loggerConfig != nil" or "loggerCore != nil && loggerWriteSyncer != nil".
	// loggerCore        zapcore.Core
	// loggerWriteSyncer zapcore.WriteSyncer
}

// Validate ensures that '*embed.Config' fields are properly configured.
func (cfg *Config) Validate() error {
	if err := cfg.setupLogging(); err != nil {
		return err
	}
	return nil
}

// GetLogger returns the logger.
func (cfg Config) GetLogger() *zap.Logger {
	cfg.loggerMu.RLock()
	l := cfg.logger
	cfg.loggerMu.RUnlock()
	return l
}

// setupLogging initializes etcd logging.
// Must be called after flag parsing or finishing configuring embed.Config.
func (cfg *Config) setupLogging() error {
	if len(cfg.LogOutputs) == 0 {
		cfg.LogOutputs = []string{DefaultLogOutput}
	}
	if len(cfg.LogOutputs) > 1 {
		for _, v := range cfg.LogOutputs {
			if v == DefaultLogOutput {
				return fmt.Errorf("multi logoutput for %q is not supported yet", DefaultLogOutput)
			}
		}
	}

	outputPaths, errOutputPaths := make([]string, 0), make([]string, 0)
	for _, v := range cfg.LogOutputs {
		switch v {
		case DefaultLogOutput:
			outputPaths = append(outputPaths, StdErrLogOutput)
			errOutputPaths = append(errOutputPaths, StdErrLogOutput)

		case StdErrLogOutput:
			outputPaths = append(outputPaths, StdErrLogOutput)
			errOutputPaths = append(errOutputPaths, StdErrLogOutput)

		case StdOutLogOutput:
			outputPaths = append(outputPaths, StdOutLogOutput)
			errOutputPaths = append(errOutputPaths, StdOutLogOutput)

		default:
			outputPaths = append(outputPaths, v)
			errOutputPaths = append(errOutputPaths, v)
		}
	}

	copied := logutil.DefaultZapLoggerConfig
	copied.OutputPaths = outputPaths
	copied.ErrorOutputPaths = errOutputPaths
	// copied = logutil.MergeOutputPaths(copied)
	copied.Level = zap.NewAtomicLevelAt(logutil.ConvertToZapLevel(cfg.LogLevel))
	if cfg.ZapLoggerBuilder == nil {
		cfg.ZapLoggerBuilder = func(c *Config) error {
			var err error
			c.logger, err = copied.Build()
			if err != nil {
				return err
			}
			zap.ReplaceGlobals(c.logger)
			c.loggerMu.Lock()
			defer c.loggerMu.Unlock()
			c.loggerConfig = &copied
			// c.loggerCore = nil
			// c.loggerWriteSyncer = nil
			return nil
		}
	}
	err := cfg.ZapLoggerBuilder(cfg)
	if err != nil {
		return err
	}
	return nil
}

func (cfg *Config) getAPURLs() (ss []string) {
	ss = make([]string, len(cfg.APUrls))
	for i := range cfg.APUrls {
		ss[i] = cfg.APUrls[i].String()
	}
	return ss
}

func (cfg *Config) getLPURLs() (ss []string) {
	ss = make([]string, len(cfg.LPUrls))
	for i := range cfg.LPUrls {
		ss[i] = cfg.LPUrls[i].String()
	}
	return ss
}

func (cfg *Config) getACURLs() (ss []string) {
	ss = make([]string, len(cfg.ACUrls))
	for i := range cfg.ACUrls {
		ss[i] = cfg.ACUrls[i].String()
	}
	return ss
}

func (cfg *Config) getLCURLs() (ss []string) {
	ss = make([]string, len(cfg.LCUrls))
	for i := range cfg.LCUrls {
		ss[i] = cfg.LCUrls[i].String()
	}
	return ss
}

// NewConfig creates a new Config populated with default values.
func NewConfig() *Config {
	lpurl, _ := url.Parse(DefaultListenPeerURLs)
	apurl, _ := url.Parse(DefaultInitialAdvertisePeerURLs)
	lcurl, _ := url.Parse(DefaultListenClientURLs)
	acurl, _ := url.Parse(DefaultAdvertiseClientURLs)
	cfg := &Config{
		Name: DefaultName,

		LPUrls: []url.URL{*lpurl},
		LCUrls: []url.URL{*lcurl},
		APUrls: []url.URL{*apurl},
		ACUrls: []url.URL{*acurl},

		InitialClusterToken: "etcd-cluster",

		loggerMu:   new(sync.RWMutex),
		logger:     nil,
		LogOutputs: []string{DefaultLogOutput},
		LogLevel:   logutil.DefaultLogLevel,
	}
	return cfg
}

// MarshalLogObject implements zapcore.ObjectMarshaller interface.
func (cfg *Config) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if cfg == nil {
		return nil
	}

	enc.AddString("Name", cfg.Name)
	enc.AddString("Dir", cfg.Dir)
	enc.AddString("WALDir", cfg.WalDir)
	enc.AddString("Cluster", cfg.InitialCluster)
	enc.AddString("ClusterToken", cfg.InitialClusterToken)
	enc.AddString("LogLevel", cfg.LogLevel)
	if err := enc.AddReflected("LogOutputs", cfg.LogOutputs); err != nil {
		return err
	}
	if err := enc.AddReflected("LPUrls", cfg.getLPURLs()); err != nil {
		return err
	}
	if err := enc.AddReflected("APUrls", cfg.getAPURLs()); err != nil {
		return err
	}
	if err := enc.AddReflected("ACUrls", cfg.getACURLs()); err != nil {
		return err
	}
	if err := enc.AddReflected("LCUrls", cfg.getLCURLs()); err != nil {
		return err
	}
	return nil
}
