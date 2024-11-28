// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/gocql/gocql"
	//"github.com/jaegertracing/jaeger/pkg/cassandra"
)

type MappingBuilder struct {
	Mode                 string
	Datacentre           string
	Keyspace             string
	Replication          string
	TraceTTL             int
	DependenciesTTL      int
	CompactionWindowSize int
	CompactionWindowUnit string
}

func (mb *MappingBuilder) renderTemplate(templatePath string) (string, error) {
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return "", err
	}

	var renderedOutput bytes.Buffer
	if err := tmpl.Execute(&renderedOutput, mb); err != nil {
		return "", err
	}

	return renderedOutput.String(), nil
}

func RenderCQLTemplate(params MappingBuilder, cqlOutput string) (string, error) {
	commentRegex := regexp.MustCompile(`--.*`)
	cqlOutput = commentRegex.ReplaceAllString(cqlOutput, "")

	lines := strings.Split(cqlOutput, "\n")
	var filteredLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}

	cqlOutput = strings.Join(filteredLines, "\n")

	replacements := map[string]string{
		"${keyspace}":               params.Keyspace,
		"${replication}":            params.Replication,
		"${trace_ttl}":              fmt.Sprintf("%d", params.TraceTTL),
		"${dependencies_ttl}":       fmt.Sprintf("%d", params.DependenciesTTL),
		"${compaction_window_size}": fmt.Sprintf("%d", params.CompactionWindowSize),
		"${compaction_window_unit}": params.CompactionWindowUnit,
	}

	for placeholder, value := range replacements {
		cqlOutput = strings.ReplaceAll(cqlOutput, placeholder, value)
	}

	return cqlOutput, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func isValidKeyspace(keyspace string) bool {
	match, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, keyspace)
	return match
}

func getCompactionWindow(traceTTL int, compactionWindow string) (int, string, error) {
	var compactionWindowSize int
	var compactionWindowUnit string

	if compactionWindow != "" {
		matched, err := regexp.MatchString(`^[0-9]+[mhd]$`, compactionWindow)
		if err != nil {
			return 0, "", err
		}

		if matched {
			numStr := strings.TrimRight(compactionWindow, "mhd")
			unitStr := strings.TrimLeft(compactionWindow, numStr)

			compactionWindowSize, err = strconv.Atoi(numStr)
			if err != nil {
				return 0, "", errors.New("invalid compaction window size format")
			}
			compactionWindowUnit = unitStr
		} else {
			return 0, "", errors.New("invalid compaction window size format. Please use numeric value followed by 'm' for minutes, 'h' for hours, or 'd' for days")
		}
	} else {
		traceTTLMinutes := traceTTL / 60
		compactionWindowSize = (traceTTLMinutes + 30 - 1) / 30
		compactionWindowUnit = "m"
	}

	switch compactionWindowUnit {
	case "m":
		compactionWindowUnit = "MINUTES"
	case "h":
		compactionWindowUnit = "HOURS"
	case "d":
		compactionWindowUnit = "DAYS"
	default:
		return 0, "", errors.New("invalid compaction window unit")
	}

	return compactionWindowSize, compactionWindowUnit, nil
}

func (mb *MappingBuilder) GetSpanServiceMappings() (spanMapping string, serviceMapping string, err error) {
	traceTTL, _ := strconv.Atoi(getEnv("TRACE_TTL", "172800"))
	dependenciesTTL, _ := strconv.Atoi(getEnv("DEPENDENCIES_TTL", "0"))
	cas_version := getEnv("VERSION", "4")
	// template := os.Args[1]
	var template string
	var cqlOutput string
	if template == "" {
		switch cas_version {
		case "3":
			template = "./v003.cql.tmpl"
		case "4":
			template = "./v004.cql.tmpl"
		default:
			template = "./v004.cql.tmpl"
		}
		mode := getEnv("MODE", "")

		if mode == "" {
			fmt.Println("missing MODE parameter")
			return
		}
		var datacentre, replications string
		// var ReplicationFactor int
		if mode == "prod" {
			datacentre = getEnv("DATACENTRE", "")
			if datacentre == "" {
				fmt.Println("missing DATACENTRE parameter for prod mode")
				return
			}
			replicationFactor, _ := strconv.Atoi(getEnv("REPLICATION_FACTOR", "2"))
			replications = fmt.Sprintf("{'class': 'NetworkTopologyStrategy', '%s': %d}", datacentre, replicationFactor)
		} else if mode == "test" {
			datacentre = getEnv("DATACENTRE", "")
			replicationFactor, _ := strconv.Atoi(getEnv("REPLICATION_FACTOR", "1"))
			replications = fmt.Sprintf("{'class': 'SimpleStrategy', 'replication_factor': '%d'}", replicationFactor)
		} else {
			fmt.Printf("invaild mode: %s, expecting 'prod' or 'test'", mode)
			return
		}
		keyspace := getEnv("KEYSPACE", fmt.Sprintf("jaeger_v1_%s", mode))
		if !isValidKeyspace(keyspace) {
			fmt.Printf("invaild characters in KEYSPACE=%s parameter , please use letters, digits, and underscores only", keyspace)
			return
		}
		CompactionWindowSize, compactionWindowUnit, _ := getCompactionWindow(traceTTL, getEnv("COMPACTION_WINDOW_SIZE", ""))

		params := MappingBuilder{
			Mode:                 mode,
			Datacentre:           datacentre,
			Keyspace:             keyspace,
			Replication:          replications,
			TraceTTL:             traceTTL,
			DependenciesTTL:      dependenciesTTL,
			CompactionWindowSize: CompactionWindowSize,
			CompactionWindowUnit: compactionWindowUnit,
		}
		// Render the template
		cqlOutput, err = params.renderTemplate(template)
		if err != nil {
			return "", "", err
		}
		cqlOutput, _ = RenderCQLTemplate(params, cqlOutput)
		// fmt.Println(cqlOutput)
	}
	return cqlOutput, "", err
}

func SchemaInit(c *gocql.Session) {
	builder := &MappingBuilder{}
	schema, _, err := builder.GetSpanServiceMappings()
	if err != nil {
		fmt.Println("Error:", err)
	}
	queries := strings.Split(schema, ";")
	for _, query := range queries {
		trimmedQuery := strings.TrimSpace(query)
		if trimmedQuery != "" {
			fmt.Println(trimmedQuery)
			if err := c.Query(trimmedQuery + ";").Exec(); err != nil {
				fmt.Printf("Failed to create sampling_probabilities table: %v", err)
			} else {
				fmt.Println("Table sampling_probabilities created successfully.")
			}
		}
	}
}
