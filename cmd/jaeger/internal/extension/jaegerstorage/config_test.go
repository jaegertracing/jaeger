package jaegerstorage

import (
	"os"
	"path/filepath"
	"testing"

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

func TestValidateNoBackends(t *testing.T) {
	conf := loadConf(t, `
backends:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	require.EqualError(t, cfg.Validate(), "at least one storage is required")
}

func TestValidateEmptyBackend(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	require.EqualError(t, cfg.Validate(), "no backend defined for storage 'some_storage'")
}

func TestUnmarshalDefaultMemory(t *testing.T) {
	conf := loadConf(t, `
backends:
  some_storage:
    memory:
`)
	cfg := createDefaultConfig().(*Config)
	require.NoError(t, conf.Unmarshal(cfg))
	require.Equal(t, 1_000_000, cfg.Backends["some_storage"].Memory.MaxTraces)
}
