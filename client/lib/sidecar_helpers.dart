/// Pure, cross-platform helpers for sidecar path, URL, and data-directory
/// resolution. Separated from `dart:io` so web builds compile cleanly and
/// unit tests can verify macOS/Windows behavior on any host OS.
library;

/// Default local PocketBase URL used by desktop builds when `API_URL` is unset.
const defaultLocalApiUrl = 'http://127.0.0.1:8090';

/// Returns the health-check URI (`<base>/api/health`) for [baseUrl].
Uri sidecarHealthUri(String baseUrl) {
  final parsed = Uri.parse(baseUrl.trim());
  final cleanPath = parsed.path.replaceAll(RegExp(r'/+$'), '');
  return parsed.replace(path: '$cleanPath/api/health');
}

/// Extracts the `host:port` listen address for PocketBase `--http=` from [baseUrl].
String sidecarListenAddress(String baseUrl) {
  final parsed = Uri.parse(baseUrl.trim());
  final host = parsed.host.isNotEmpty ? parsed.host : '127.0.0.1';
  final port = parsed.hasPort
      ? parsed.port
      : (parsed.scheme == 'https' ? 443 : 8090);
  final formattedHost = host.contains(':') ? '[$host]' : host;
  return '$formattedHost:$port';
}

/// Returns `true` when [baseUrl] targets a loopback interface that can be
/// backed by a local desktop sidecar.
bool isLocalSidecarTarget(String baseUrl) {
  final parsed = Uri.tryParse(baseUrl.trim());
  if (parsed == null || !parsed.hasAuthority) return false;
  final host = parsed.host.toLowerCase();
  return host == '127.0.0.1' || host == 'localhost' || host == '::1';
}

/// Resolves ordered candidate paths for the bundled `versionradar-server`
/// binary on macOS or Windows.
List<String> resolveSidecarExecutableCandidates({
  required String resolvedExecutable,
  required bool isMacOS,
  required bool isWindows,
  String? currentDirectory,
  String? pathEnv,
}) {
  if (!isMacOS && !isWindows) return const [];

  final candidates = <String>[];
  final seen = <String>{};

  void addCandidate(String path) {
    if (path.isEmpty) return;
    if (seen.add(path)) {
      candidates.add(path);
    }
  }

  if (isMacOS) {
    const binName = 'versionradar-server';
    final exeDir = _dirnamePosix(resolvedExecutable);
    if (exeDir.isNotEmpty) {
      // Standard macOS bundle: Contents/MacOS/<exe> -> Contents/Resources/<bin>
      addCandidate(_normalizePosix('$exeDir/../Resources/$binName'));

      // Also handle explicit Contents/Resources lookup if executable sits deeper.
      final contentsIdx = resolvedExecutable.indexOf('/Contents/');
      if (contentsIdx != -1) {
        final bundleContents = resolvedExecutable.substring(
          0,
          contentsIdx + '/Contents'.length,
        );
        addCandidate('$bundleContents/Resources/$binName');
      }

      addCandidate('$exeDir/$binName');
    }

    if (currentDirectory != null && currentDirectory.trim().isNotEmpty) {
      final cwd = _trimTrailingSeparators(currentDirectory.trim(), '/');
      addCandidate('$cwd/$binName');
    }

    if (pathEnv != null && pathEnv.isNotEmpty) {
      for (final dir in pathEnv.split(':')) {
        final trimmed = _trimTrailingSeparators(dir.trim(), '/');
        if (trimmed.isNotEmpty) {
          addCandidate('$trimmed/$binName');
        }
      }
    }
  } else if (isWindows) {
    const binName = 'versionradar-server.exe';
    final exeDir = _dirnameWindows(resolvedExecutable);
    if (exeDir.isNotEmpty) {
      addCandidate('$exeDir\\data\\$binName');
      addCandidate('$exeDir\\$binName');
    }

    if (currentDirectory != null && currentDirectory.trim().isNotEmpty) {
      final cwd = _trimTrailingSeparators(
        currentDirectory.trim().replaceAll('/', '\\'),
        '\\',
      );
      addCandidate('$cwd\\$binName');
    }

    if (pathEnv != null && pathEnv.isNotEmpty) {
      for (final dir in pathEnv.split(';')) {
        final trimmed = _trimTrailingSeparators(
          dir.trim().replaceAll('/', '\\'),
          '\\',
        );
        if (trimmed.isNotEmpty) {
          addCandidate('$trimmed\\$binName');
        }
      }
    }
  }

  return candidates;
}

/// Resolves the platform-appropriate persistent `pb_data` directory for the
/// sidecar server, or `null` if required environment variables are unavailable.
String? resolveSidecarDataDir({
  required bool isMacOS,
  required bool isWindows,
  required Map<String, String> environment,
}) {
  if (isMacOS) {
    final home = environment['HOME']?.trim();
    if (home == null || home.isEmpty) return null;
    final cleanHome = _trimTrailingSeparators(home, '/');
    return '$cleanHome/Library/Application Support/VersionRadar/pb_data';
  }
  if (isWindows) {
    final appData = _firstNonEmpty([
      environment['APPDATA'],
      environment['LOCALAPPDATA'],
      environment['USERPROFILE'],
    ]);
    if (appData == null) return null;
    final cleanBase = _trimTrailingSeparators(
      appData.replaceAll('/', '\\'),
      '\\',
    );
    return '$cleanBase\\VersionRadar\\pb_data';
  }
  return null;
}

String? _firstNonEmpty(List<String?> values) {
  for (final v in values) {
    final trimmed = v?.trim();
    if (trimmed != null && trimmed.isNotEmpty) return trimmed;
  }
  return null;
}

String _trimTrailingSeparators(String path, String sep) {
  var out = path;
  while (out.length > 1 && out.endsWith(sep)) {
    out = out.substring(0, out.length - 1);
  }
  return out;
}

String _dirnamePosix(String path) {
  final idx = path.lastIndexOf('/');
  if (idx < 0) return '';
  if (idx == 0) return '/';
  return path.substring(0, idx);
}

String _dirnameWindows(String path) {
  final normalized = path.replaceAll('/', '\\');
  final idx = normalized.lastIndexOf('\\');
  if (idx < 0) return '';
  if (idx == 2 && normalized.length >= 3 && normalized[1] == ':') {
    return normalized.substring(0, 3);
  }
  return normalized.substring(0, idx);
}

String _normalizePosix(String path) {
  final isAbsolute = path.startsWith('/');
  final parts = <String>[];
  for (final segment in path.split('/')) {
    if (segment.isEmpty || segment == '.') continue;
    if (segment == '..') {
      if (parts.isNotEmpty && parts.last != '..') {
        parts.removeLast();
      } else if (!isAbsolute) {
        parts.add('..');
      }
    } else {
      parts.add(segment);
    }
  }
  final joined = parts.join('/');
  return isAbsolute ? '/$joined' : joined;
}
