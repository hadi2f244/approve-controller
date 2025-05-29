package consts

import (
	"fmt"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	operatorConfigPathKey                      = "operator.config.path"
	lookupRequeueAfterTimeSecond               = "operator.config.lookupRequeueAfterTimeSecond"
	logLevelKey                                = "log.level"
	operatorCalicoNetworkPolicyExcludedListKey = "operator.caliconetworkpolicy.excludedList"
)

var (
	defaultLogLevel                                = "info"
	defaultOperatorConfigPathValue                 = "/etc/operator-config/config.yaml"
	defaultOperatorCalicoNetworkPolicyExcludedList = []string{"kube-system", "calico-system", "calico-apiserver", "kube-node-lease", "ingress-nginx"}
	defaultLookupRequeueAfterTimeSecond            = int64(30 * time.Second)
)

type Configuration struct {
	v *viper.Viper
}

func getOperatorConfigPath() (string, error) {
	var operatorConfigPathEnv = "OPERATOR_CONFIG_PATH"
	operatorConfigPath, found := os.LookupEnv(operatorConfigPathEnv)
	if !found {
		return "", fmt.Errorf("Loading config from %s", operatorConfigPathEnv)
	}
	return operatorConfigPath, nil
}

func NewConfiguration() (*Configuration, error) {
	c := Configuration{
		v: viper.New(),
	}

	// Set default
	c.v.SetDefault(logLevelKey, defaultLogLevel)
	c.v.SetDefault(operatorCalicoNetworkPolicyExcludedListKey, defaultOperatorCalicoNetworkPolicyExcludedList)
	c.v.SetDefault(lookupRequeueAfterTimeSecond, defaultLookupRequeueAfterTimeSecond)
	c.v.SetDefault(operatorConfigPathKey, defaultOperatorConfigPathValue)
	if operatorConfigPath, err := getOperatorConfigPath(); err != nil {
		c.v.SetDefault(operatorConfigPathKey, operatorConfigPath)
	}
	c.v.AutomaticEnv()
	c.v.SetConfigFile(c.GetPathToConfig())
	err := c.v.ReadInConfig() // Find and read the config file
	logrus.WithField("path", c.GetPathToConfig()).Warn("loading config")
	if _, ok := err.(*os.PathError); ok {
		logrus.Warnf("no config file '%s' not found. Using default values", c.GetPathToConfig())
	} else if err != nil { // Handle other errors that occurred while reading the config file
		logrus.WithField("configLoader", err).Warn("fatal error while reading the config file")
		return nil, fmt.Errorf("fatal error while reading the config file: %s", err)
	}
	setLogLevel(c.GetLogLevel())
	c.v.WatchConfig()
	c.v.OnConfigChange(func(e fsnotify.Event) {
		logrus.WithField("file", e.Name).Warn("Config file changed")
		setLogLevel(c.GetLogLevel())
	})
	return &c, nil
}

// GetLogLevel returns the log level
func (c *Configuration) GetLogLevel() string {
	s := c.v.GetString(logLevelKey)
	return s
}

// GetPathToConfig returns the path to the config file
func (c *Configuration) GetPathToConfig() string {
	return c.v.GetString(operatorConfigPathKey)
}

func (c *Configuration) GetLookupRequeueAfterTimeSecond() time.Duration {
	return time.Duration(c.v.GetInt64(lookupRequeueAfterTimeSecond))
}

func (c *Configuration) GetOperatorCalicoNetworkPolicyExcludedList() []string {
	logrus.WithField("operatorCalicoNetworkPolicyExcludedListKey", c.v.GetStringSlice(operatorCalicoNetworkPolicyExcludedListKey)).Warn("excluded namespaces")
	return c.v.GetStringSlice(operatorCalicoNetworkPolicyExcludedListKey)
}

func setLogLevel(logLevel string) {
	logrus.WithField("level", logLevel).Warn("setting log level")
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.WithField("level", logLevel).Fatalf("failed to start: %s", err.Error())
	}
	logrus.SetLevel(level)
}
