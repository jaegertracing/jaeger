CREATE TABLE
    IF NOT EXISTS {{ .Table }} AS {{ .Table }}_local ENGINE = Distributed(
        '{{ .Cluster }}',
        '{{ .Database }}',
        '{{ .Table }}_local'{{ if .ShardingKey }},
        {{ .ShardingKey }}{{ end }}
    )
