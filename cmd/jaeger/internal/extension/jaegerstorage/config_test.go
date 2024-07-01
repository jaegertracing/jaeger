package jaegerstorage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"
)

func loadConf(t *testing.T, config string) *confmap.Conf {
	d := t.TempDir()
	f := filepath.Join(d, "config.yaml")
	require.NoError(t, os.WriteFile(f, []byte(config), 0644))
	cm, err := confmaptest.LoadConf(f)
	require.NoError(t, err)
	return cm
}

func TestConfigValidateNoBackends(t *testing.T) {
	conf := loadConf(t, `
backends:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	require.EqualError(t, cfg.Validate(), "at least one storage is required")
}

func TestConfigValidateEmptyBackend(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	require.EqualError(t, cfg.Validate(), "no backend defined for storage 'some_storage'")
}

func TestConfigDefaultMemory(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
    memory:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	assert.NotEmpty(t, cfg.Backends["some_storage"].Memory.MaxTraces)
}

func TestConfigDefaultBadger(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
    badger:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	assert.NotEmpty(t, cfg.Backends["some_storage"].Badger.SpanStoreTTL)
}

func TestConfigDefaultGRPC(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
    grpc:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	assert.NotEmpty(t, cfg.Backends["some_storage"].GRPC.Timeout)
}
