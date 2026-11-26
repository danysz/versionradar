import 'package:pocketbase/pocketbase.dart';
import 'package:shared_preferences/shared_preferences.dart';

const apiUrl = String.fromEnvironment(
  'API_URL',
  defaultValue: 'http://127.0.0.1:8090',
);

late final PocketBase pb;

Future<void> initApi() async {
  final prefs = await SharedPreferences.getInstance();
  pb = PocketBase(
    apiUrl,
    authStore: AsyncAuthStore(
      save: (data) async => prefs.setString('pb_auth', data),
      initial: prefs.getString('pb_auth'),
    ),
  );
}

/// Versioned custom API, e.g. api('/plugins').
Future<dynamic> api(
  String path, {
  String method = 'GET',
  Map<String, dynamic>? body,
}) => pb.send('/api/v1$path', method: method, body: body ?? const {});

class SyncResult {
  const SyncResult({
    required this.events,
    required this.sources,
    required this.problems,
  });

  final int events;
  final int sources;
  final List<String> problems;

  String get summary {
    if (problems.isNotEmpty) {
      return problems.length == 1
          ? problems.first
          : 'Synced with ${problems.length} issues';
    }
    if (sources > 0) {
      return 'Synced $events deployment${events == 1 ? '' : 's'} · $sources Git source${sources == 1 ? '' : 's'}';
    }
    return 'Synced $events deployment${events == 1 ? '' : 's'}';
  }
}

/// Pulls from providers and Git sources now (live updates arrive via PocketBase realtime).
Future<SyncResult> syncNow() async {
  final r = await api('/sync', method: 'POST') as Map<String, dynamic>;
  return SyncResult(
    events: (r['events'] as num?)?.toInt() ?? 0,
    sources: (r['sources'] as num?)?.toInt() ?? 0,
    problems: ((r['problems'] as List?) ?? const []).cast<String>(),
  );
}

Future<Map<String, dynamic>> fetchPluginsCatalog() async {
  return (await api('/plugins')) as Map<String, dynamic>;
}

Future<Map<String, dynamic>> fetchWebhookPlans() async {
  return (await api('/webhooks/plans')) as Map<String, dynamic>;
}

Future<Map<String, dynamic>> connectRadarWebhook(
  String radarId, {
  String kind = 'provider',
  Map<String, dynamic>? overrideConfig,
}) async {
  return (await api(
        '/radars/$radarId/webhook/connect',
        method: 'POST',
        body: {
          'kind': kind,
          if (overrideConfig != null && overrideConfig.isNotEmpty)
            'override_config': overrideConfig,
        },
      ))
      as Map<String, dynamic>;
}

Future<Map<String, dynamic>> connectAllWebhooks({
  bool force = false,
  Map<String, Map<String, dynamic>>? overrides,
}) async {
  return (await api(
        '/webhooks/connect-all',
        method: 'POST',
        body: {
          'force': force,
          if (overrides != null && overrides.isNotEmpty) 'overrides': overrides,
        },
      ))
      as Map<String, dynamic>;
}
