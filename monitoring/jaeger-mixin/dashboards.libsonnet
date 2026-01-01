local g = (import 'grafana-builder/grafana.libsonnet') + {
  qpsPanelErrTotal(selectorErr, selectorTotal):: {
    local expr(selector) = 'sum(rate(' + selector + '[1m]))',

    aliasColors: {
      success: '#7EB26D',
      'error': '#E24D42',
    },
    targets: [
      {
        expr: expr(selectorErr),
        format: 'time_series',
        intervalFactor: 2,
        legendFormat: 'error',
        refId: 'A',
        step: 10,
      },
      {
        expr: expr(selectorTotal) + ' - ' + expr(selectorErr),
        format: 'time_series',
        intervalFactor: 2,
        legendFormat: 'success',
        refId: 'B',
        step: 10,
      },
    ],
  } + $.stack,

  qpsPanelSuccessError(selectorErr, selectorSuccess):: {
    local expr(selector) = 'sum(rate(' + selector + '[1m]))',

    aliasColors: {
      success: '#7EB26D',
      'error': '#E24D42',
    },
    targets: [
      {
        expr: expr(selectorErr),
        format: 'time_series',
        intervalFactor: 2,
        legendFormat: 'error',
        refId: 'A',
        step: 10,
      },
      {
        expr: expr(selectorSuccess),
        format: 'time_series',
        intervalFactor: 2,
        legendFormat: 'success',
        refId: 'B',
        step: 10,
      },
    ],
  } + $.stack,
};

{
  grafanaDashboards+: {
    'jaeger.json':
      g.dashboard('Jaeger')
      .addRow(
        g.row('Services')
        .addPanel(
          g.panel('span creation rate') +
          g.qpsPanelErrTotal('jaeger_tracer_reporter_spans_total{result=~"dropped|err"}', 'jaeger_tracer_reporter_spans_total') +
          g.stack
        )
        .addPanel(
          g.panel('% spans dropped') +
          g.queryPanel('sum(rate(jaeger_tracer_reporter_spans_total{result=~"dropped|err"}[1m])) by (namespace) / sum(rate(jaeger_tracer_reporter_spans_total[1m])) by (namespace)', '{{namespace}}') +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Agent')
        .addPanel(
          g.panel('batch ingest rate') +
          g.qpsPanelErrTotal('jaeger_agent_reporter_batches_failures_total', 'jaeger_agent_reporter_batches_submitted_total') +
          g.stack
        )
        .addPanel(
          g.panel('% batches dropped') +
          g.queryPanel('sum(rate(jaeger_agent_reporter_batches_failures_total[1m])) by (cluster) / sum(rate(jaeger_agent_reporter_batches_submitted_total[1m])) by (cluster)', '{{cluster}}') +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Collector')
        .addPanel(
          g.panel('span ingest rate') +
          g.qpsPanelErrTotal('jaeger_collector_spans_dropped_total', 'jaeger_collector_spans_received_total') +
          g.stack
        )
        .addPanel(
          g.panel('% spans dropped') +
          g.queryPanel('sum(rate(jaeger_collector_spans_dropped_total[1m])) by (instance) / sum(rate(jaeger_collector_spans_received_total[1m])) by (instance)', '{{instance}}') +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Collector Queue')
        .addPanel(
          g.panel('span queue length') +
          g.queryPanel('jaeger_collector_queue_length', '{{instance}}') +
          g.stack
        )
        .addPanel(
          g.panel('span queue time - 95 percentile') +
          g.queryPanel('histogram_quantile(0.95, sum(rate(jaeger_collector_in_queue_latency_bucket[1m])) by (le, instance))', '{{instance}}')
        )
      )
      .addRow(
        g.row('Query')
        .addPanel(
          g.panel('qps') +
          g.qpsPanelErrTotal('jaeger_query_requests_total{result="err"}', 'jaeger_query_requests_total') +
          g.stack
        )
        .addPanel(
          g.panel('latency - 99 percentile') +
          g.queryPanel('histogram_quantile(0.99, sum(rate(jaeger_query_latency_bucket[1m])) by (le, instance))', '{{instance}}') +
          g.stack
        )
      ),
    'jaeger-v2.json':
      g.dashboard('Jaeger V2')
      .addRow(
        g.row('Collector (Receivers)')
        .addPanel(
          g.panel('span ingest rate') +
          g.qpsPanelSuccessError('otelcol_receiver_refused_spans', 'otelcol_receiver_accepted_spans') +
          g.stack
        )
        .addPanel(
          g.panel('% spans dropped') +
          g.queryPanel('sum(rate(otelcol_receiver_refused_spans[1m])) by (receiver) / (sum(rate(otelcol_receiver_accepted_spans[1m])) by (receiver) + sum(rate(otelcol_receiver_refused_spans[1m])) by (receiver))', '{{receiver}}') +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Exporters')
        .addPanel(
          g.panel('span export rate') +
          g.qpsPanelSuccessError('otelcol_exporter_send_failed_spans', 'otelcol_exporter_sent_spans') +
          g.stack
        )
        .addPanel(
          g.panel('% spans dropped') +
          g.queryPanel('sum(rate(otelcol_exporter_send_failed_spans[1m])) by (exporter) / (sum(rate(otelcol_exporter_sent_spans[1m])) by (exporter) + sum(rate(otelcol_exporter_send_failed_spans[1m])) by (exporter))', '{{exporter}}') +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) } +
          g.stack
        )
      )
      .addRow(
        g.row('Queue')
        .addPanel(
          g.panel('queue size') +
          g.queryPanel('otelcol_exporter_queue_size', '{{exporter}}') +
          g.stack
        )
        .addPanel(
          g.panel('latency - 50 percentile') +
          g.queryPanel('histogram_quantile(0.50, sum(rate(otelcol_exporter_send_duration_bucket[1m])) by (le, exporter))', '{{exporter}}') +
          g.stack
        )
        .addPanel(
          g.panel('latency - 95 percentile') +
          g.queryPanel('histogram_quantile(0.95, sum(rate(otelcol_exporter_send_duration_bucket[1m])) by (le, exporter))', '{{exporter}}') +
          g.stack
        )
        .addPanel(
          g.panel('latency - 99 percentile') +
          g.queryPanel('histogram_quantile(0.99, sum(rate(otelcol_exporter_send_duration_bucket[1m])) by (le, exporter))', '{{exporter}}') +
          g.stack
        )
      ),
  },
}
