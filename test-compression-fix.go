// Test program to verify OpenSearch compression fix
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()

	// Test configuration with compression enabled
	cfg := &config.Configuration{
		Servers:              []string{"http://localhost:9200"},
		Version:              0, // Auto-detect
		CreateIndexTemplates: true,
		HTTPCompression:      true, // This triggers the bug
	}

	ctx := context.Background()

	fmt.Println("=== OpenSearch Compression Fix Test ===")
	fmt.Println("Configuration:")
	fmt.Printf("- Servers: %v\n", cfg.Servers)
	fmt.Printf("- HTTPCompression: %v\n", cfg.HTTPCompression)
	fmt.Printf("- CreateIndexTemplates: %v\n", cfg.CreateIndexTemplates)
	fmt.Println()

	// Create client
	client, err := config.NewClient(ctx, cfg, logger, nil)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Check if it's OpenSearch
	fmt.Println("Checking server type...")

	// The actual ping and version detection happens inside NewClient
	// If this is OpenSearch and compression is enabled, the template
	// creation should work with our fix

	fmt.Println("Client created successfully!")

	// Try to create a test template
	testTemplate := `{
		"index_patterns": ["test-jaeger-*"],
		"settings": {
			"number_of_shards": 1,
			"number_of_replicas": 0
		}
	}`

	fmt.Println("\nTrying to create test template...")
	resp, err := client.CreateTemplate("test-jaeger").Body(testTemplate).Do(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Compressor detection") {
			fmt.Println("❌ BUG REPRODUCED: Template creation failed with compression error")
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Template creation failed with different error: %v\n", err)
		}
	} else {
		fmt.Println("✅ SUCCESS: Template created successfully with compression enabled!")
		fmt.Printf("Response: %+v\n", resp)
	}
}
