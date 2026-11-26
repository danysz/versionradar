import 'dart:convert';

import 'package:pocketbase/pocketbase.dart';

/// Clean, concise display formatting so UI never shows raw UUIDs, long walls of text,
/// or noisy provider/exception strings.
class DisplayFormat {
  static final _uuidRe = RegExp(
    r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
  );
  static final _hexShaRe = RegExp(r'^[0-9a-fA-F]{12,40}$');

  /// Shortens raw version strings (e.g. 40-char SHAs, UUIDs, or "1.2.3 (45)") into a crisp label
  /// plus an optional secondary build/meta tag.
  static ({String primary, String? meta}) version(String raw) {
    final s = raw.trim();
    if (s.isEmpty) return (primary: '—', meta: null);
    if (_uuidRe.hasMatch(s)) return (primary: '#${s.substring(0, 8)}', meta: null);
    if (_hexShaRe.hasMatch(s)) return (primary: s.substring(0, 7), meta: null);

    // Split "1.2.3 (45)" -> primary: "1.2.3", meta: "build 45"
    final paren = RegExp(r'^([^\s(]+)\s*\(([^)]+)\)$').firstMatch(s);
    if (paren != null) {
      return (primary: paren.group(1)!, meta: paren.group(2)!);
    }
    // Split "1.2.3+45" -> primary: "1.2.3", meta: "+45"
    final plus = s.indexOf('+');
    if (plus > 0 && plus < s.length - 1) {
      return (primary: s.substring(0, plus), meta: '+${s.substring(plus + 1)}');
    }
    if (s.length > 22) {
      return (primary: '${s.substring(0, 20)}…', meta: null);
    }
    return (primary: s, meta: null);
  }

  /// Turns raw provider stage/state strings into readable labels.
  static String stage(String raw) {
    final s = raw.trim();
    if (s.isEmpty) return '';
    final upper = s.toUpperCase();
    const known = {
      'QUEUED': 'Queued',
      'PENDING': 'Pending',
      'INITIALIZE': 'Initializing',
      'CLONE_REPO': 'Cloning Repo',
      'BUILD': 'Building',
      'BUILDING': 'Building',
      'BUILT': 'Built',
      'BUILD_FAILED': 'Build Failed',
      'DEPLOY': 'Deploying',
      'DEPLOYED': 'Deployed',
      'RELEASE': 'Releasing',
      'RELEASING': 'Releasing',
      'RELEASED': 'Released',
      'RELEASE_FAILED': 'Release Failed',
      'COMPLETED': 'Completed',
      'WORKING': 'Building',
      'DISTRIBUTED': 'Distributed',
      'LIVE': 'Live',
      'APPROVED': 'Approved',
      'IN_REVIEW': 'In Review',
      'WAITING_FOR_REVIEW': 'Waiting for Review',
      'PROCESSING': 'Processing',
      'PREPARING': 'Preparing',
      'READY_FOR_SALE': 'Live on App Store',
      'READY_FOR_DISTRIBUTION': 'Ready',
      'REJECTED': 'Rejected',
      'INVALID_BINARY': 'Invalid Binary',
      'STARTED': 'Started',
    };
    if (known.containsKey(upper)) return known[upper]!;
    // Handle track:status like "production:completed" or "step (2/5)"
    final cleaned = s
        .replaceAll(':', ' · ')
        .replaceAll('_', ' ')
        .replaceAll('-', ' ');
    return cleaned[0].toUpperCase() + cleaned.substring(1);
  }

  /// Produces a clean, concise 1-line summary without redundant prefixes or multi-line commit dumps.
  static ({String? env, String? branch, String text}) deploymentMessage(
    String raw,
    String platform,
  ) {
    var line = raw.split('\n').first.trim();
    if (line.isEmpty) return (env: null, branch: null, text: '');

    String? env;
    String? branch;

    // Cloudflare pattern: "production main: fix login"
    if (platform == 'cloudflare') {
      final m = RegExp(
        r'^(production|preview)\s+([^:\s]+)(?::\s*(.*))?$',
        caseSensitive: false,
      ).firstMatch(line);
      if (m != null) {
        env = _capitalize(m.group(1)!);
        branch = m.group(2);
        line = (m.group(3) ?? '').trim();
      }
    }

    // Strip noisy boilerplate like "Cloud Build SUCCESS" or "appStoreVersion...: READY_FOR_SALE"
    if (RegExp(r'^Cloud Build\s+[A-Z_]+$').hasMatch(line)) {
      line = '';
    } else if (line.startsWith('Heroku build ') ||
        line.startsWith('Release v')) {
      line = '';
    } else if (line.contains('appStoreVersion')) {
      final parts = line.split(':');
      line = parts.length > 1 ? stage(parts.last) : '';
    } else if (RegExp(r'^[A-Z][A-Z0-9_]{3,}$').hasMatch(line)) {
      line = stage(line);
    }

    if (line.length > 90) {
      line = '${line.substring(0, 88)}…';
    }
    return (env: env, branch: branch, text: line);
  }

  /// Extracts a short human-readable target name from a radar's target JSON (e.g. "my-app", "com.acme.app").
  static String radarTargetLabel(RecordModel radar) {
    final raw = radar.getStringValue('target').trim();
    if (raw.isEmpty) return '';
    try {
      final m = (jsonDecode(raw) as Map).cast<String, dynamic>();
      for (final key in [
        'project',
        'app_name',
        'package_name',
        'app_id',
        'project_id',
      ]) {
        final v = (m[key] as String?)?.trim() ?? '';
        if (v.isNotEmpty) return v;
      }
    } catch (_) {}
    return '';
  }

  /// Extracts a short repo name from a radar's source_target JSON (e.g. "org/repo").
  static String sourceTargetLabel(RecordModel radar) {
    final raw = radar.getStringValue('source_target').trim();
    if (raw.isEmpty) return '';
    try {
      final m = (jsonDecode(raw) as Map).cast<String, dynamic>();
      for (final key in ['repo', 'project', 'repo_url']) {
        var v = (m[key] as String?)?.trim() ?? '';
        if (v.isEmpty) continue;
        if (v.startsWith('http')) {
          final uri = Uri.tryParse(v);
          if (uri != null && uri.path.length > 1) {
            v = uri.path.substring(1).replaceAll(RegExp(r'\.git$'), '');
          }
        }
        return v;
      }
    } catch (_) {}
    return '';
  }

  /// Formats a Git ref like "tag v1.2.3", "branch main", or "pubspec.yaml@main" into a tiny badge label.
  static String shortRef(String raw) {
    final s = raw.trim();
    if (s.isEmpty) return '';
    if (s.startsWith('tag ')) return 'tag';
    if (s.startsWith('branch ')) return s.substring(7);
    if (s.contains('@')) return s.split('@').first;
    return s.length > 16 ? '${s.substring(0, 14)}…' : s;
  }

  /// Converts PocketBase ClientException or Dart exceptions into clean short messages.
  static String friendlyError(Object err) {
    if (err is ClientException) {
      final msg = err.response['message'] as String?;
      if (msg != null && msg.trim().isNotEmpty) {
        return msg.trim();
      }
      if (err.statusCode == 0) {
        return 'Unable to reach server. Check your connection.';
      }
      return 'Request failed (HTTP ${err.statusCode})';
    }
    var s = err.toString().replaceFirst(RegExp(r'^Exception:\s*'), '').trim();
    if (s.length > 140) s = '${s.substring(0, 138)}…';
    return s;
  }

  /// Shortens a URL for compact display while keeping full copyable text.
  static String compactUrl(String url) {
    final u = url.trim();
    if (u.length <= 48) return u;
    return '${u.substring(0, 28)}…${u.substring(u.length - 16)}';
  }

  static String _capitalize(String s) =>
      s.isEmpty ? s : s[0].toUpperCase() + s.substring(1).toLowerCase();
}
