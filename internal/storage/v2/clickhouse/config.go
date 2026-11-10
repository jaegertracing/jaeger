// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

const (
	defaultProtocol                      = "native"
	defaultDatabase                      = "jaeger"
	defaultSearchDepth                   = 1000
	defaultMaxSearchDepth                = 10000
	defaultAttributeMetadataCacheTTL     = time.Hour
	defaultAttributeMetadataCacheMaxSize = 1000
)

type Configuration struct {
	// Protocol is the protocol to use to connect to ClickHouse.
	// Supported values are "native" and "http". Default is "native".
	Protocol string `mapstructure:"protocol" valid:"in(native|http),optional"`
	// Addresses contains a list of ClickHouse server addresses to connect to.
	Addresses []string `mapstructure:"addresses" valid:"required"`
	// Database is the ClickHouse database to connect to.
	Database string `mapstructure:"database"`
	// Auth contains the authentication configuration to connect to ClickHouse.
	Auth Authentication `mapstructure:"auth"`
	// TLS, when present, enables TLS for the ClickHouse connection.
	TLS configoptional.Optional[configtls.ClientConfig] `mapstructure:"tls"`
	// DialTimeout is the timeout for establishing a connection to ClickHouse.
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
	// CreateSchema, if set to true, will create the ClickHouse schema if it does not exist.
	// It requires TableEngine to name the engine family the tables are created with.
	CreateSchema bool `mapstructure:"create_schema"`
	// TableEngine selects the engine family for every table CreateSchema creates.
	// Exactly one of its variants must be set when CreateSchema is true; the block is
	// ignored otherwise, because the operator then owns the tables.
	TableEngine TableEngine `mapstructure:"table_engine"`
	// DefaultSearchDepth is the default search depth for queries.
	// This is the maximum number of trace IDs that will be returned when searching for traces
	// if a limit is not specified in the query.
	DefaultSearchDepth uint32 `mapstructure:"default_search_depth"`
	// MaxSearchDepth is the maximum allowed search depth for queries.
	// This limits the number of trace IDs that can be returned when searching for traces.
	MaxSearchDepth uint32 `mapstructure:"max_search_depth"`
	// AttributeMetadataCacheTTL is the time-to-live for cached attribute metadata entries.
	// Attribute metadata maps attribute keys to their stored types and levels,
	// which is needed to build type-correct queries for querying attributes.
	// Default is 1h. 0 means cached entries never expire.
	AttributeMetadataCacheTTL time.Duration `mapstructure:"attribute_metadata_cache_ttl"`
	// AttributeMetadataCacheMaxSize is the maximum number of entries in the attribute metadata cache.
	// Default is 1000. 0 disables caching.
	AttributeMetadataCacheMaxSize int `mapstructure:"attribute_metadata_cache_max_size"`
	// TTL is the Time-To-Live for spans in the database.
	// Data older than this will be automatically deleted. 0 means disabled.
	TTL time.Duration `mapstructure:"ttl"`
	// HoistedAttributes lists the attributes that the spans table copies into dedicated
	// MATERIALIZED columns. CreateSchema adds the columns to a table it creates; an existing
	// table is not altered, so the operator adds the columns to it. An empty list leaves the
	// schema as it is.
	HoistedAttributes []HoistedAttribute `mapstructure:"hoisted_attributes"`
}

// HoistedAttribute selects one attribute to copy into a column of its own. All fields except
// Codec are required.
type HoistedAttribute struct {
	// Key is the attribute key. It is an opaque string, not a column name or an expression.
	Key string `mapstructure:"key"`
	// Level is where the attribute is stored: "span" or "resource".
	Level string `mapstructure:"level"`
	// Type is the stored type of the attribute: "string", "int", "double" or "bool". It selects
	// the typed group to copy from and is not a conversion: a value stored under the same key
	// with another type is not copied.
	Type string `mapstructure:"type"`
	// Column is the name of the dedicated column. It must match [a-z_][a-z0-9_]* and must not
	// be a column the spans table already has.
	Column string `mapstructure:"column"`
	// Codec is the compression codec of the column, in the form ZSTD(level) with a level from
	// 1 through 22. The default is ZSTD(1).
	Codec string `mapstructure:"codec"`
}

// hoistedColumn is a validated HoistedAttribute.
type hoistedColumn struct {
	attribute chtracestore.HoistedAttribute
	codec     string
}

// definition returns the column definition as it appears in CREATE TABLE. The name is quoted
// because a name such as index is a keyword at that position.
func (c hoistedColumn) definition() string {
	return fmt.Sprintf("`%s` %s MATERIALIZED %s CODEC(%s)",
		c.attribute.Column, c.attribute.ColumnType(), c.attribute.SourceExpression(), c.codec)
}

const defaultHoistedCodec = "ZSTD(1)"

var (
	hoistedColumnNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	hoistedCodecPattern      = regexp.MustCompile(`^ZSTD\((\d{1,2})\)$`)
	// spansColumnPattern matches the name of a top-level column in the spans table template,
	// which is how a hoisted column is checked against the columns the table already has.
	spansColumnPattern = regexp.MustCompile(`(?m)^ {8}([a-z_]+) `)

	hoistedLevels = map[string]expression.Level{
		"span":     expression.LevelSpan,
		"resource": expression.LevelResource,
	}
	hoistedTypes = map[string]pcommon.ValueType{
		"string": pcommon.ValueTypeStr,
		"int":    pcommon.ValueTypeInt,
		"double": pcommon.ValueTypeDouble,
		"bool":   pcommon.ValueTypeBool,
	}
)

// zstdCodec returns codec as ClickHouse reports it, and false unless codec is ZSTD(level) with
// a level that ClickHouse accepts. No other codec is accepted, so the value can be rendered
// into DDL without being arbitrary SQL. The level is rendered from its number because
// ClickHouse stores ZSTD(01) as ZSTD(1), and the startup check compares the stored text.
func zstdCodec(codec string) (string, bool) {
	m := hoistedCodecPattern.FindStringSubmatch(codec)
	if m == nil {
		return "", false
	}
	// The pattern admits one or two digits, so the conversion cannot fail.
	level, _ := strconv.Atoi(m[1])
	if level < 1 || level > 22 {
		return "", false
	}
	return fmt.Sprintf("ZSTD(%d)", level), true
}

// hoistedColumns validates HoistedAttributes and returns the columns they describe, in the
// configured order. Validation and conversion share one function so that no column can be
// rendered from an entry that was not checked.
func (cfg *Configuration) hoistedColumns() ([]hoistedColumn, error) {
	type source struct {
		level expression.Level
		key   string
		typ   pcommon.ValueType
	}
	sources := make(map[source]struct{}, len(cfg.HoistedAttributes))
	names := make(map[string]struct{})
	for _, match := range spansColumnPattern.FindAllStringSubmatch(sql.CreateSpansTable, -1) {
		names[match[1]] = struct{}{}
	}
	columns := make([]hoistedColumn, 0, len(cfg.HoistedAttributes))
	for i, h := range cfg.HoistedAttributes {
		fail := func(format string, args ...any) ([]hoistedColumn, error) {
			return nil, fmt.Errorf("hoisted_attributes[%d]: %s", i, fmt.Sprintf(format, args...))
		}
		if h.Key == "" {
			return fail("key must not be empty")
		}
		level, ok := hoistedLevels[h.Level]
		if !ok {
			return fail("level must be span or resource, not %q", h.Level)
		}
		typ, ok := hoistedTypes[h.Type]
		if !ok {
			return fail("type must be string, int, double or bool, not %q", h.Type)
		}
		if !hoistedColumnNamePattern.MatchString(h.Column) {
			return fail("column %q must match [a-z_][a-z0-9_]*", h.Column)
		}
		if _, taken := names[h.Column]; taken {
			return fail("column %q is already a column of the spans table or of another entry", h.Column)
		}
		names[h.Column] = struct{}{}
		src := source{level: level, key: h.Key, typ: typ}
		if _, dup := sources[src]; dup {
			return fail("the %s attribute %q of type %s is listed twice", h.Level, h.Key, h.Type)
		}
		sources[src] = struct{}{}
		codec := defaultHoistedCodec
		if h.Codec != "" {
			codec, ok = zstdCodec(h.Codec)
		}
		if !ok {
			return fail("codec %q must be ZSTD(level) with a level from 1 through 22", h.Codec)
		}
		columns = append(columns, hoistedColumn{
			attribute: chtracestore.HoistedAttribute{
				Level:  level,
				Key:    h.Key,
				Type:   typ,
				Column: h.Column,
			},
			codec: codec,
		})
	}
	return columns, nil
}

type Authentication struct {
	Basic configoptional.Optional[basicauthextension.ClientAuthSettings] `mapstructure:"basic"`
}

// TableEngine selects the ClickHouse engine family for the tables the backend creates.
// The backend knows which tables are `MergeTree` and which are `AggregatingMergeTree`, so the
// choice here is only between the local engines and their Replicated counterparts.
type TableEngine struct {
	// MergeTree creates local tables, which hold data only on the server that received the
	// insert. It is the right choice for a single-node server.
	MergeTree configoptional.Optional[MergeTreeEngine] `mapstructure:"merge_tree"`
	// Replicated creates `ReplicatedMergeTree` and `ReplicatedAggregatingMergeTree` tables, which
	// keep every row on every replica of the shard. It is required on a cluster with more than
	// one replica.
	Replicated configoptional.Optional[ReplicatedEngine] `mapstructure:"replicated"`
}

// MergeTreeEngine has no parameters.
type MergeTreeEngine struct{}

// ReplicatedEngine carries the two arguments of a Replicated* engine. Both are optional,
// but only together: when both are empty the engine is rendered without arguments and the
// server's `default_replica_path` and `default_replica_name` settings apply, and setting one
// without the other is a validation error. Both are passed to ClickHouse verbatim, so
// server macros such as `{shard}`, `{replica}`, `{database}` and `{table}` are substituted by the
// server.
type ReplicatedEngine struct {
	// KeeperPath is the path in ClickHouse Keeper under which the table's replication
	// metadata is kept. The same template is used for every table in the schema, so it
	// must contain the `{table}` macro, or another expansion distinct per table, because
	// ClickHouse refuses two tables under one path.
	KeeperPath string `mapstructure:"keeper_path"`
	// ReplicaName identifies this replica under KeeperPath. The same value is used for
	// every table, which is what the `{replica}` macro expects.
	ReplicaName string `mapstructure:"replica_name"`
}

// engineFamily is the local engine of a table in the schema, which a TableEngine
// renders either as is or as its Replicated counterpart.
type engineFamily string

const (
	mergeTreeFamily            engineFamily = "MergeTree"
	aggregatingMergeTreeFamily engineFamily = "AggregatingMergeTree"
)

// engineClause validates the TableEngine and returns the body of the ENGINE clause for a
// table of the given family, as the configured TableEngine renders it. Validation and
// rendering share one function so that no clause can be rendered from a configuration
// that was not checked.
func (e TableEngine) engineClause(family engineFamily) (string, error) {
	hasMergeTree := e.MergeTree.HasValue()
	hasReplicated := e.Replicated.HasValue()
	if hasMergeTree && hasReplicated {
		return "", errors.New("`table_engine` must set only one of `merge_tree` or `replicated`")
	}
	if !hasMergeTree && !hasReplicated {
		return "", errors.New("`create_schema` requires `table_engine` with exactly one of `merge_tree` or `replicated`: " +
			"use `merge_tree` for a single-node server, `replicated` for a cluster with more than one replica, " +
			"or set `create_schema: false` and manage the tables yourself")
	}
	r := e.Replicated.Get()
	if r == nil {
		return string(family), nil
	}
	if (r.KeeperPath == "") != (r.ReplicaName == "") {
		return "", errors.New("`table_engine.replicated` must set `keeper_path` and `replica_name` together or neither")
	}
	// Both values are rendered inside single-quoted SQL literals, where a quote ends the
	// literal and a backslash escapes the character after it.
	if strings.ContainsAny(r.KeeperPath, `'\`) || strings.ContainsAny(r.ReplicaName, `'\`) {
		return "", errors.New("`table_engine.replicated` `keeper_path` and `replica_name` must not contain a single quote or a backslash")
	}
	if r.KeeperPath == "" {
		return "Replicated" + string(family), nil
	}
	return fmt.Sprintf("Replicated%s('%s', '%s')", family, r.KeeperPath, r.ReplicaName), nil
}

// DefaultConfiguration returns the configuration a ClickHouse backend starts from
// before the user's own settings are unmarshaled over it.
func DefaultConfiguration() Configuration {
	return Configuration{
		Protocol:                      defaultProtocol,
		Database:                      defaultDatabase,
		DefaultSearchDepth:            defaultSearchDepth,
		MaxSearchDepth:                defaultMaxSearchDepth,
		AttributeMetadataCacheTTL:     defaultAttributeMetadataCacheTTL,
		AttributeMetadataCacheMaxSize: defaultAttributeMetadataCacheMaxSize,
	}
}

func (cfg *Configuration) Validate() error {
	if _, err := govalidator.ValidateStruct(cfg); err != nil {
		return err
	}
	if cfg.TTL < 0 {
		return errors.New("ttl must be a non-negative duration")
	}
	if cfg.TTL > 0 && cfg.TTL%time.Second != 0 {
		return errors.New("ttl must be a whole number of seconds")
	}
	// A search depth of zero would make every trace search return nothing, and a
	// negative one is meaningless, so reject both rather than querying with them.
	if cfg.DefaultSearchDepth == 0 {
		return errors.New("default_search_depth must be a positive number")
	}
	if cfg.MaxSearchDepth == 0 {
		return errors.New("max_search_depth must be a positive number")
	}
	if cfg.DefaultSearchDepth > cfg.MaxSearchDepth {
		return errors.New("default_search_depth cannot exceed max_search_depth")
	}
	if cfg.DialTimeout < 0 {
		return errors.New("dial_timeout must be a non-negative duration")
	}
	if cfg.AttributeMetadataCacheTTL < 0 {
		return errors.New("attribute_metadata_cache_ttl must be a non-negative duration")
	}
	if cfg.AttributeMetadataCacheMaxSize < 0 {
		return errors.New("attribute_metadata_cache_max_size must be a non-negative number")
	}
	if _, err := cfg.hoistedColumns(); err != nil {
		return err
	}
	// The table_engine block is ignored without create_schema, because the operator then
	// owns the tables.
	if cfg.CreateSchema {
		if _, err := cfg.TableEngine.engineClause(mergeTreeFamily); err != nil {
			return err
		}
	}
	return nil
}
