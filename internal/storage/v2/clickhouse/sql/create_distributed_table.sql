CREATE TABLE
    IF NOT EXISTS {{ .Table }} AS {{ .LocalTable }} ENGINE = Distributed(
        '{{ .Cluster }}',
        currentDatabase(),
        '{{ .LocalTable }}'{{ if .ShardingKey }},
        {{ .ShardingKey }}{{ end }}
    )
