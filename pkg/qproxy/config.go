package qproxy

import (
	"errors"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// SetCommandFlags creates flags and bind them to Viper.
func SetCommandFlags(fs *pflag.FlagSet, v *viper.Viper) {
	fs.StringP("config-file", "c", "qproxy.yaml", "Configuration file")

	fs.VisitAll(func(f *pflag.Flag) {
		v.BindPFlag(strings.ReplaceAll(f.Name, "-", "_"), fs.Lookup(f.Name))
	})
}

// InitConfig reads in config file
func InitConfig(v *viper.Viper) error {
	configFile := v.GetString("config_file")
	v.SetConfigName(filepath.Base(configFile))
	v.AddConfigPath(filepath.Dir(configFile))
	v.SetConfigType("yaml")

	return v.ReadInConfig()
}

type backendConfig struct {
	url         string
	sessionTTL  time.Duration
	maxSessions int
	tlsInsecure bool
	weight      float64
}

type proxyConfig struct {
	m                    sync.Map
	v                    *viper.Viper
	reloadLock           sync.Mutex
	reloadNotifyChanList []chan struct{}
}

func newQProxyConfig(v *viper.Viper) (*proxyConfig, error) {
	if err := ValidateProxyConfig(v); err != nil {
		return nil, fmt.Errorf("Invalid QProxy configuration: `%s`", err)
	}

	config := proxyConfig{
		v:                    v,
		reloadNotifyChanList: make([]chan struct{}, 0),
	}
	if err := config.loadDynamicConfig(); err != nil {
		return nil, fmt.Errorf("Invalid QProxy configuration: `%s`", err)
	}

	config.m.Store("addr", v.GetString("addr"))
	config.m.Store("cookie_name", v.GetString("cookie_name"))
	config.m.Store("timeout", v.GetDuration("timeout")*time.Second)
	config.m.Store("tls.cert_file", v.GetString("tls.cert_file"))
	config.m.Store("tls.key_file", v.GetString("tls.key_file"))
	config.m.Store("api.addr", v.GetString("api.addr"))
	config.m.Store("api.tls.cert_file", v.GetString("api.tls.cert_file"))
	config.m.Store("api.tls.key_file", v.GetString("api.tls.key_file"))

	return &config, nil
}

func (c *proxyConfig) getValue(key string) interface{} {
	value, ok := c.m.Load(key)
	if !ok {
		log.WithFields(log.Fields{"key": key}).Fatal("Unknown configuration key")
	}

	return value
}

func (c *proxyConfig) getString(key string) string {
	return c.getValue(key).(string)
}

func (c *proxyConfig) getDuration(key string) time.Duration {
	return c.getValue(key).(time.Duration)
}

func (c *proxyConfig) getIPList(key string) *ipList {
	return c.getValue(key).(*ipList)
}

func (c *proxyConfig) getInt(key string) int {
	return c.getValue(key).(int)
}

func (c *proxyConfig) getTemplate(key string) *template.Template {
	return c.getValue(key).(*template.Template)
}

func (c *proxyConfig) getBackendsConfig() map[string]*backendConfig {
	return c.getValue("backends_config_map").(map[string]*backendConfig)
}

func (c *proxyConfig) loadDynamicConfig() error {
	trustedProxies, err := newIPList(c.v.GetStringSlice("trusted_proxies"))
	if err != nil {
		return err
	}

	whitelistedIps, err := newIPList(c.v.GetStringSlice("whitelisted_ips"))
	if err != nil {
		return err
	}

	queueTemplate, err := template.ParseFiles(c.v.GetString("queue.template"))
	if err != nil {
		return err
	}

	fullQueueTemplate, err := template.ParseFiles(c.v.GetString("queue.full_template"))
	if err != nil {
		return err
	}

	backendsConfigMap := make(map[string]*backendConfig)
	for backendName := range c.v.GetStringMap("backends") {
		rawBackendConfig := c.v.Sub("backends." + backendName)
		backendsConfigMap[backendName] = &backendConfig{
			url:         rawBackendConfig.GetString("url"),
			sessionTTL:  rawBackendConfig.GetDuration("session_ttl") * time.Second,
			maxSessions: rawBackendConfig.GetInt("max_sessions"),
			tlsInsecure: rawBackendConfig.GetBool("tls.insecure"),
			weight:      rawBackendConfig.GetFloat64("weight"),
		}
	}

	c.m.Store("trusted_proxies", trustedProxies)
	c.m.Store("whitelisted_ips", whitelistedIps)
	c.m.Store("session_refresh_interval", c.v.GetDuration("session_refresh_interval")*time.Second)
	c.m.Store("queue.session_ttl", c.v.GetDuration("queue.session_ttl")*time.Second)
	c.m.Store("queue.max_sessions", c.v.GetInt("queue.max_sessions"))
	c.m.Store("queue.template", queueTemplate)
	c.m.Store("queue.full_template", fullQueueTemplate)
	c.m.Store("backends_config_map", backendsConfigMap)
	c.m.Store("api.username", c.v.GetString("api.username"))
	c.m.Store("api.password", c.v.GetString("api.password"))

	return nil
}

func (c *proxyConfig) syncReload() error {
	c.reloadLock.Lock()
	defer c.reloadLock.Unlock()

	if err := c.v.ReadInConfig(); err != nil {
		return err
	}

	if err := ValidateProxyConfig(c.v); err != nil {
		return err
	}

	if err := c.loadDynamicConfig(); err != nil {
		return err
	}

	for _, notifyChan := range c.reloadNotifyChanList {
		notifyChan <- struct{}{}
	}

	return nil
}

func (c *proxyConfig) reloadNotifyChan() chan struct{} {
	reloadNotifyChan := make(chan struct{})
	c.reloadLock.Lock()
	c.reloadNotifyChanList = append(c.reloadNotifyChanList, reloadNotifyChan)
	c.reloadLock.Unlock()

	return reloadNotifyChan
}

// ValidateProxyConfig validate a viper instance.
func ValidateProxyConfig(v *viper.Viper) error {
	requiredStringOptions := []string{"addr", "cookie_name", "queue.template",
		"queue.full_template", "api.addr",
	}
	requiredDurationOptions := []string{"session_refresh_interval", "queue.session_ttl", "timeout"}

	for _, key := range requiredStringOptions {
		if v.GetString(key) == "" {
			return fmt.Errorf("Missing `%s` option", key)
		}
	}

	for _, key := range requiredDurationOptions {
		if v.GetDuration(key) == time.Duration(0) {
			return fmt.Errorf("Option `%s` must be greater than 0", key)
		}
	}

	if v.GetInt("queue.max_sessions") < 0 {
		return errors.New("Option `queue.max_sessions` must be greater or equals than 0")
	}

	if len(v.GetStringMap("backends")) == 0 {
		return errors.New("No backends available")
	}

	for backendName := range v.GetStringMap("backends") {
		backendConfig := v.Sub("backends." + backendName)
		if err := validateBackendConfig(backendConfig); err != nil {
			return fmt.Errorf("[backend: %s] %s", backendName, err)
		}
	}

	return nil
}

func validateBackendConfig(v *viper.Viper) error {
	v.SetDefault("weight", 1)

	if v.GetString("url") == "" {
		return errors.New("Missing `url` option")
	}

	if v.GetDuration("session_ttl") == time.Duration(0) {
		return errors.New("Option `session_ttl` must be greater than 0")
	}

	if v.GetInt("max_sessions") == 0 {
		return errors.New("Option `max_sessions` must be greater than 0")
	}

	if v.GetFloat64("weight") == 0 {
		return errors.New("Option `weight` must be greater than 0")
	}

	if v.GetFloat64("weight") > 1 {
		return errors.New("Option `weight` must be less or equals than 1")
	}

	return nil
}
