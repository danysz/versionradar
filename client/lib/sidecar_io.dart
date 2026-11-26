import 'dart:async';
import 'dart:io';
import 'dart:ui';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

import 'sidecar_helpers.dart';

/// Callback type for probing the backend health endpoint.
typedef SidecarHealthProbe = Future<bool> Function(Uri healthUri);

/// Callback type for launching the sidecar process.
typedef SidecarProcessStarter =
    Future<Process> Function(String executable, List<String> arguments);

/// Manages the lifecycle of the bundled `versionradar-server` desktop sidecar.
class SidecarManager {
  SidecarManager({
    SidecarHealthProbe? healthProbe,
    SidecarProcessStarter? processStarter,
    bool Function(String path)? fileExists,
    Future<void> Function(String path)? ensureDir,
    String Function()? resolvedExecutable,
    String Function()? currentDirectory,
    Map<String, String> Function()? environment,
    bool? isMacOS,
    bool? isWindows,
    bool? isWeb,
    bool registerHooks = true,
  }) : _healthProbe = healthProbe ?? _defaultHealthProbe,
       _processStarter = processStarter ?? _defaultProcessStarter,
       _fileExists = fileExists ?? ((p) => File(p).existsSync()),
       _ensureDir = ensureDir ?? _defaultEnsureDir,
       _resolvedExecutable =
           resolvedExecutable ?? (() => Platform.resolvedExecutable),
       _currentDirectory = currentDirectory ?? (() => Directory.current.path),
       _environment = environment ?? (() => Platform.environment),
       _isMacOS = isMacOS ?? Platform.isMacOS,
       _isWindows = isWindows ?? Platform.isWindows,
       _isWeb = isWeb ?? kIsWeb,
       _registerHooksEnabled = registerHooks;

  /// Shared singleton used by app startup and lifecycle hooks.
  static final SidecarManager instance = SidecarManager();

  final SidecarHealthProbe _healthProbe;
  final SidecarProcessStarter _processStarter;
  final bool Function(String path) _fileExists;
  final Future<void> Function(String path) _ensureDir;
  final String Function() _resolvedExecutable;
  final String Function() _currentDirectory;
  final Map<String, String> Function() _environment;
  final bool _isMacOS;
  final bool _isWindows;
  final bool _isWeb;
  final bool _registerHooksEnabled;

  Process? _process;
  Future<void>? _startupFuture;
  bool _hooksRegistered = false;
  AppLifecycleListener? _lifecycleListener;
  StreamSubscription<ProcessSignal>? _sigintSub;
  StreamSubscription<ProcessSignal>? _sigtermSub;
  String? _lastError;

  /// Whether a sidecar process launched by this manager is currently active.
  bool get isRunning => _process != null;

  /// Last non-fatal startup error message, if any.
  String? get lastError => _lastError;

  bool get _isSupportedDesktop => !_isWeb && (_isMacOS || _isWindows);

  /// Ensures a local backend is reachable at [baseUrl], starting the bundled
  /// sidecar binary if needed and waiting up to [healthTimeout] for `/api/health`.
  Future<void> ensureStarted({
    String baseUrl = defaultLocalApiUrl,
    Duration healthTimeout = const Duration(seconds: 5),
  }) {
    return _startupFuture ??= _ensureStartedImpl(
      baseUrl: baseUrl,
      healthTimeout: healthTimeout,
    ).whenComplete(() {
      _startupFuture = null;
    });
  }

  Future<void> _ensureStartedImpl({
    required String baseUrl,
    required Duration healthTimeout,
  }) async {
    _lastError = null;
    if (!_isSupportedDesktop || !isLocalSidecarTarget(baseUrl)) {
      return;
    }
    if (_process != null) {
      return;
    }

    final healthUri = sidecarHealthUri(baseUrl);
    if (await _healthProbe(healthUri)) {
      return;
    }

    final env = _environment();
    final exePath = _findExecutable(env);
    if (exePath == null) {
      _lastError = 'Bundled versionradar-server binary was not found.';
      debugPrint('[sidecar] $_lastError');
      return;
    }

    final dataDir = resolveSidecarDataDir(
      isMacOS: _isMacOS,
      isWindows: _isWindows,
      environment: env,
    );
    if (dataDir == null) {
      _lastError = 'Could not resolve application data directory for pb_data.';
      debugPrint('[sidecar] $_lastError');
      return;
    }

    await _ensureDir(dataDir);
    if (_registerHooksEnabled) {
      _registerHooks();
    }

    final listenAddr = sidecarListenAddress(baseUrl);
    late final Process proc;
    try {
      proc = await _processStarter(exePath, [
        'serve',
        '--http=$listenAddr',
        '--dir=$dataDir',
        '--no-open',
      ]);
    } catch (e) {
      _lastError = 'Failed to start sidecar process: $e';
      debugPrint('[sidecar] $_lastError');
      return;
    }

    _process = proc;
    var exitedEarly = false;
    int? exitCode;

    // Drain stdout/stderr so child pipes cannot fill up and block the server.
    unawaited(proc.stdout.drain<void>().catchError((_) {}));
    unawaited(proc.stderr.drain<void>().catchError((_) {}));
    unawaited(
      proc.exitCode
          .then((code) {
            exitedEarly = true;
            exitCode = code;
            if (identical(_process, proc)) {
              _process = null;
            }
          })
          .catchError((_) {
            exitedEarly = true;
            if (identical(_process, proc)) {
              _process = null;
            }
          }),
    );

    final deadline = DateTime.now().add(healthTimeout);
    while (DateTime.now().isBefore(deadline)) {
      if (exitedEarly) {
        _lastError = 'Sidecar exited prematurely (code ${exitCode ?? -1}).';
        debugPrint('[sidecar] $_lastError');
        return;
      }
      if (await _healthProbe(healthUri)) {
        return;
      }
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }

    _lastError =
        'Timed out after ${healthTimeout.inSeconds}s waiting for $healthUri.';
    debugPrint('[sidecar] $_lastError');
    await stop();
  }

  /// Stops the managed sidecar process if it was started by this manager.
  Future<void> stop() async {
    final proc = _process;
    if (proc == null) return;
    _process = null;

    try {
      proc.kill(ProcessSignal.sigterm);
      await proc.exitCode.timeout(
        const Duration(seconds: 2),
        onTimeout: () {
          proc.kill(ProcessSignal.sigkill);
          return -1;
        },
      );
    } catch (_) {
      // Process may have already exited.
    }
  }

  /// Releases lifecycle and signal subscriptions (useful in tests).
  Future<void> dispose() async {
    await stop();
    _lifecycleListener?.dispose();
    _lifecycleListener = null;
    await _sigintSub?.cancel();
    _sigintSub = null;
    await _sigtermSub?.cancel();
    _sigtermSub = null;
    _hooksRegistered = false;
  }

  String? _findExecutable(Map<String, String> env) {
    final candidates = resolveSidecarExecutableCandidates(
      resolvedExecutable: _resolvedExecutable(),
      isMacOS: _isMacOS,
      isWindows: _isWindows,
      currentDirectory: _currentDirectory(),
      pathEnv: env['PATH'] ?? env['Path'],
    );
    for (final candidate in candidates) {
      if (_fileExists(candidate)) {
        return candidate;
      }
    }
    return null;
  }

  void _registerHooks() {
    if (_hooksRegistered) return;
    _hooksRegistered = true;

    try {
      _lifecycleListener = AppLifecycleListener(
        onExitRequested: () async {
          await stop();
          return AppExitResponse.exit;
        },
        onDetach: () {
          unawaited(stop());
        },
      );
    } catch (e) {
      debugPrint('[sidecar] AppLifecycleListener unavailable: $e');
    }

    try {
      _sigintSub = ProcessSignal.sigint.watch().listen((_) async {
        await stop();
        exit(0);
      });
    } catch (e) {
      debugPrint('[sidecar] SIGINT watch unavailable: $e');
    }

    if (!_isWindows) {
      try {
        _sigtermSub = ProcessSignal.sigterm.watch().listen((_) async {
          await stop();
          exit(0);
        });
      } catch (e) {
        debugPrint('[sidecar] SIGTERM watch unavailable: $e');
      }
    }
  }

  static Future<bool> _defaultHealthProbe(Uri healthUri) async {
    final client = HttpClient();
    client.connectionTimeout = const Duration(milliseconds: 500);
    try {
      final req = await client
          .getUrl(healthUri)
          .timeout(const Duration(milliseconds: 500));
      final res = await req.close().timeout(const Duration(milliseconds: 500));
      await res.drain<void>();
      return res.statusCode == 200;
    } catch (_) {
      return false;
    } finally {
      client.close(force: true);
    }
  }

  static Future<Process> _defaultProcessStarter(
    String executable,
    List<String> arguments,
  ) => Process.start(executable, arguments);

  static Future<void> _defaultEnsureDir(String path) =>
      Directory(path).create(recursive: true);
}

/// Ensures the desktop sidecar is running before API initialization.
Future<void> ensureSidecarStarted({
  required String baseUrl,
  Duration healthTimeout = const Duration(seconds: 5),
}) => SidecarManager.instance.ensureStarted(
  baseUrl: baseUrl,
  healthTimeout: healthTimeout,
);

/// Terminates the managed desktop sidecar process if running.
Future<void> stopSidecar() => SidecarManager.instance.stop();

/// Whether the managed sidecar process is currently running.
bool get isSidecarRunning => SidecarManager.instance.isRunning;

/// Last non-fatal sidecar startup error, if any.
String? get sidecarLastError => SidecarManager.instance.lastError;
