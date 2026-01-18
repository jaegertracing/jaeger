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
