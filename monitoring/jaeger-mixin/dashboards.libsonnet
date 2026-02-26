local g = (import 'grafana-builder/grafana.libsonnet');

{
  grafanaDashboards+: {
    'jaeger.json':
      g.dashboard('Jaeger v2')
      .addRow(
        g.row('Collector - Ingestion')
        .addPanel(
          g.panel('Span Ingest Rate') +
          g.queryPanel(
            [
              'sum(rate(otelcol_receiver_refused_spans_total[1m])) or vector(0)',
              'sum(rate(otelcol_receiver_accepted_spans_total[1m]))',
            ],
            [
              'error',
              'success',
            ]
          ) +
          g.stack
        )
        .addPanel(
          g.panel('% Spans Refused') +
          g.queryPanel(
            'sum(rate(otelcol_receiver_refused_spans_total[1m])) by (receiver, transport) / (sum(rate(otelcol_receiver_accepted_spans_total[1m])) by (receiver, transport) + sum(rate(otelcol_receiver_refused_spans_total[1m])) by (receiver, transport)) or vector(0)',
            '{{receiver}}-{{transport}}'
          ) +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Collector - Export')
        .addPanel(
          g.panel('Span Export Rate') +
          g.queryPanel(
            [
              'sum(rate(otelcol_exporter_send_failed_spans_total[1m])) or vector(0)',
              'sum(rate(otelcol_exporter_sent_spans_total[1m]))',
            ],
            [
              'error',
              'success',
            ]
          ) +
          g.stack
        )
        .addPanel(
          g.panel('Export Success Rate %') +
          g.queryPanel(
            '(sum(rate(otelcol_exporter_sent_spans_total[1m])) by (exporter) / (sum(rate(otelcol_exporter_sent_spans_total[1m])) by (exporter) + sum(rate(otelcol_exporter_send_failed_spans_total[1m])) by (exporter))) * 100 or vector(0)',
            '{{exporter}}'
          ) +
          { yaxes: g.yaxes({ format: 'percent', max: 100 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Storage')
        .addPanel(
          g.panel('Storage Request Rate') +
          g.queryPanel(
            'sum(rate(jaeger_storage_requests_total[1m])) by (operation, result)',
            '{{operation}} - {{result}}'
          ) +
          g.stack
        )
        .addPanel(
          g.panel('Storage Latency - P99') +
          g.queryPanel(
            'histogram_quantile(0.99, sum(rate(jaeger_storage_latency_seconds_bucket[1m])) by (le, operation))',
            '{{operation}}'
          ) +
          { yaxes: g.yaxes({ format: 's' }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Query')
        .addPanel(
          g.panel('Query Request Rate') +
          g.queryPanel(
            'sum(rate(http_server_request_duration_seconds_count{http_route="/api/traces"}[1m])) by (http_response_status_code)',
            'status {{http_response_status_code}}'
          ) +
          g.stack
        )
        .addPanel(
          g.panel('Query Latency - P99') +
          g.queryPanel(
            'histogram_quantile(0.99, sum(rate(http_server_request_duration_seconds_bucket{http_route="/api/traces"}[1m])) by (le))',
            'P99'
          ) +
          { yaxes: g.yaxes({ format: 's' }) } +
          g.stack
        )
      )
      .addRow(
        g.row('System')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel(
            'rate(otelcol_process_cpu_seconds_total[1m])',
            'CPU'
          ) +
          { yaxes: g.yaxes({ format: 'percentunit' }) } +
          g.stack
        )
        .addPanel(
          g.panel('Memory RSS') +
          g.queryPanel(
            'otelcol_process_memory_rss_bytes',
            'Memory'
          ) +
          { yaxes: g.yaxes({ format: 'bytes' }) } +
          g.stack
        )
      ),
  },
}