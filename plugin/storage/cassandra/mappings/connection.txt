package main2fefwsef

import (
    "fmt"
    "log"

    "github.com/gocql/gocql"
)

func main() {
    // Replace with your Cassandra contact points
    cluster := gocql.NewCluster("127.0.0.1")
    cluster.Keyspace = "jaeger_v1_test" // Optional: Set default keyspace
    session, err := cluster.CreateSession()
    if err != nil {
        log.Fatal(err)
    }
    defer session.Close()

    // Your schema creation logic goes here
	createSamplingProbabilities := `
  CREATE TABLE IF NOT EXISTS jaeger_v1_test.sampling_probabilities (
	  bucket        int,
	  ts            timeuuid,
	  hostname      text,
	  probabilities text,
	  PRIMARY KEY(bucket, ts)
  ) WITH CLUSTERING ORDER BY (ts desc);`
	if err := session.Query(createSamplingProbabilities).Exec(); err != nil {
        log.Fatalf("Failed to create sampling_probabilities table: %v", err)
    } else {
        fmt.Println("Table sampling_probabilities created successfully.")
    } 

}
