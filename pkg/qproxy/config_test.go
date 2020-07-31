package qproxy

import (
	"fmt"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestSetCommandFlags(t *testing.T) {
	v := viper.New()
	fs := pflag.NewFlagSet("test", pflag.PanicOnError)
	SetCommandFlags(fs, v)

	assert.Subset(t, v.AllKeys(), []string{"config_file"})
}

func TestMissingConfig(t *testing.T) {
	err := ValidateProxyConfig(viper.New())
	assert.EqualError(t, err, "Missing `addr` option")
}

func TestMissingRequiredOptions(t *testing.T) {
	v := viper.New()
	requiredStringOptions := []string{"addr", "cookie_name", "queue.template",
		"queue.full_template", "api.addr",
	}
	requiredDurationOptions := []string{"session_refresh_interval", "queue.session_ttl", "timeout"}

	for _, key := range requiredStringOptions {
		err := ValidateProxyConfig(v)
		assert.EqualError(t, err, fmt.Sprintf("Missing `%s` option", key))
		v.Set(key, "foo")
	}

	for _, key := range requiredDurationOptions {
		err := ValidateProxyConfig(v)
		assert.EqualError(t, err, fmt.Sprintf("Option `%s` must be greater than 0", key))
		v.Set(key, 1)
	}
}

func TestNegativeMaxSessions(t *testing.T) {
	v := newViper()
	v.Set("queue.max_sessions", -1)
	err := ValidateProxyConfig(v)
	assert.EqualError(t, err, "Option `queue.max_sessions` must be greater or equals than 0")
}

func TestMissingBackends(t *testing.T) {
	err := ValidateProxyConfig(newViper())
	assert.EqualError(t, err, "No backends available")
}

func TestBackendConfig(t *testing.T) {
	v := newViper()

	v.Set("backends", map[string]interface{}{"a": map[string]interface{}{}})
	assert.EqualError(t, ValidateProxyConfig(v), "[backend: a] Missing `url` option")

	v.Set("backends.a.url", "foo")
	assert.EqualError(t, ValidateProxyConfig(v), "[backend: a] Option `session_ttl` must be greater than 0")

	v.Set("backends.a.session_ttl", 1)
	assert.EqualError(t, ValidateProxyConfig(v), "[backend: a] Option `max_sessions` must be greater than 0")

	v.Set("backends.a.max_sessions", 1)
	v.Set("backends.a.weight", 0)
	assert.EqualError(t, ValidateProxyConfig(v), "[backend: a] Option `weight` must be greater than 0")

	v.Set("backends.a.weight", 2)
	assert.EqualError(t, ValidateProxyConfig(v), "[backend: a] Option `weight` must be less or equals than 1")
}

func newViper() *viper.Viper {
	v := viper.New()
	v.Set("addr", testAddr)
	v.Set("cookie_name", "qpid")
	v.Set("queue.template", "../../test/template.html")
	v.Set("queue.full_template", "../../test/template.html")
	v.Set("api.addr", apiAddr)
	v.Set("session_refresh_interval", 1)
	v.Set("queue.session_ttl", 5)
	v.Set("timeout", 5)

	return v
}
