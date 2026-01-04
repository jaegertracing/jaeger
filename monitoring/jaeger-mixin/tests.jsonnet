local dashboard = (import 'mixin.libsonnet').grafanaDashboards['jaeger.json'];

{
  testTitle: std.assertEqual(dashboard.title, 'Jaeger V2'),
  testPanelsCount: std.assertEqual(std.length(dashboard.rows), 3),
}
