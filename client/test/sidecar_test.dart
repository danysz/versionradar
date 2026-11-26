import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:versionradar/api.dart';
import 'package:versionradar/sidecar.dart';
import 'package:versionradar/sidecar_io.dart';

class _FakeProcess implements Process {
  final _stdoutController = StreamController<List<int>>();
  final _stderrController = StreamController<List<int>>();
  final _exitCompleter = Completer<int>();
  final List<ProcessSignal> killSignals = [];

  @override
  Future<int> get exitCode => _exitCompleter.future;

  @override
  int get pid => 4242;

  @override
  IOSink get stdin => throw UnimplementedError();

  @override
  Stream<List<int>> get stdout => _stdoutController.stream;

  @override
  Stream<List<int>> get stderr => _stderrController.stream;

  @override
  bool kill([ProcessSignal signal = ProcessSignal.sigterm]) {
    killSignals.add(signal);
    completeExit(0);
    return true;
  }

  void completeExit(int code) {
    if (!_exitCompleter.isCompleted) {
      _exitCompleter.complete(code);
    }
    _stdoutController.close();
    _stderrController.close();
  }
}

void main() {
  group('resolveApiUrl', () {
    test('uses current origin on web when API_URL is unset', () {
      expect(
        resolveApiUrl(
          isWeb: true,
          baseUri: Uri.parse('https://radar.example.com/app/#/login'),
        ),
        'https://radar.example.com',
      );
    });

    test('falls back to loopback on desktop', () {
      expect(resolveApiUrl(isWeb: false), 'http://127.0.0.1:18899');
    });
  });

  group('sidecar helpers', () {
    test('computes health URI, listen address, and local target check', () {
      expect(
        sidecarHealthUri('http://127.0.0.1:18899').toString(),
        'http://127.0.0.1:18899/api/health',
      );
      expect(
        sidecarHealthUri('http://localhost:9090/').toString(),
        'http://localhost:9090/api/health',
      );
      expect(sidecarListenAddress('http://127.0.0.1:18899'), '127.0.0.1:18899');
      expect(sidecarListenAddress('http://127.0.0.1'), '127.0.0.1:18899');
      expect(sidecarListenAddress('http://localhost:9100'), 'localhost:9100');
      expect(isLocalSidecarTarget('http://127.0.0.1:18899'), isTrue);
      expect(isLocalSidecarTarget('http://localhost:18899'), isTrue);
      expect(isLocalSidecarTarget('https://radar.example.com'), isFalse);
    });

    test('resolves macOS bundle, cwd, and PATH candidates in priority order', () {
      final candidates = resolveSidecarExecutableCandidates(
        resolvedExecutable:
            '/Applications/VersionRadar.app/Contents/MacOS/VersionRadar',
        isMacOS: true,
        isWindows: false,
        currentDirectory: '/tmp/work',
        pathEnv: '/usr/local/bin:/opt/homebrew/bin',
      );
      expect(candidates, [
        '/Applications/VersionRadar.app/Contents/Resources/versionradar-server',
        '/Applications/VersionRadar.app/Contents/MacOS/versionradar-server',
        '/tmp/work/versionradar-server',
        '/usr/local/bin/versionradar-server',
        '/opt/homebrew/bin/versionradar-server',
      ]);
    });

    test('resolves Windows data/, exe dir, cwd, and PATH candidates', () {
      final candidates = resolveSidecarExecutableCandidates(
        resolvedExecutable: r'C:\Program Files\VersionRadar\versionradar.exe',
        isMacOS: false,
        isWindows: true,
        currentDirectory: r'C:\dev',
        pathEnv: r'C:\tools;C:\bin',
      );
      expect(candidates, [
        r'C:\Program Files\VersionRadar\data\versionradar-server.exe',
        r'C:\Program Files\VersionRadar\versionradar-server.exe',
        r'C:\dev\versionradar-server.exe',
        r'C:\tools\versionradar-server.exe',
        r'C:\bin\versionradar-server.exe',
      ]);
    });

    test('resolves macOS and Windows pb_data directories', () {
      expect(
        resolveSidecarDataDir(
          isMacOS: true,
          isWindows: false,
          environment: {'HOME': '/Users/alice/'},
        ),
        '/Users/alice/Library/Application Support/VersionRadar/pb_data',
      );
      expect(
        resolveSidecarDataDir(
          isMacOS: false,
          isWindows: true,
          environment: {'APPDATA': r'C:\Users\alice\AppData\Roaming'},
        ),
        r'C:\Users\alice\AppData\Roaming\VersionRadar\pb_data',
      );
    });
  });

  group('SidecarManager', () {
    test('skips spawning when backend is already healthy', () async {
      var started = false;
      final manager = SidecarManager(
        isWeb: false,
        isMacOS: true,
        isWindows: false,
        registerHooks: false,
        healthProbe: (_) async => true,
        processStarter: (_, _) async {
          started = true;
          return _FakeProcess();
        },
      );

      await manager.ensureStarted(baseUrl: 'http://127.0.0.1:18899');
      expect(started, isFalse);
      expect(manager.isRunning, isFalse);
      expect(manager.lastError, isNull);
    });

    test('is a no-op on web or non-local targets', () async {
      var probed = false;
      final webManager = SidecarManager(
        isWeb: true,
        isMacOS: false,
        isWindows: false,
        registerHooks: false,
        healthProbe: (_) async {
          probed = true;
          return false;
        },
      );
      await webManager.ensureStarted(baseUrl: 'http://127.0.0.1:18899');
      expect(probed, isFalse);

      final remoteManager = SidecarManager(
        isWeb: false,
        isMacOS: true,
        isWindows: false,
        registerHooks: false,
        healthProbe: (_) async {
          probed = true;
          return false;
        },
      );
      await remoteManager.ensureStarted(baseUrl: 'https://radar.example.com');
      expect(probed, isFalse);
    });

    test('starts bundled binary, waits for /api/health, and stops cleanly', () async {
      final fakeProc = _FakeProcess();
      var probeCount = 0;
      String? startedExe;
      List<String>? startedArgs;
      String? createdDir;

      final manager = SidecarManager(
        isWeb: false,
        isMacOS: true,
        isWindows: false,
        registerHooks: false,
        resolvedExecutable: () =>
            '/Applications/VersionRadar.app/Contents/MacOS/VersionRadar',
        currentDirectory: () => '/tmp',
        environment: () => {'HOME': '/Users/alice', 'PATH': '/usr/bin'},
        fileExists: (p) =>
            p ==
            '/Applications/VersionRadar.app/Contents/Resources/versionradar-server',
        ensureDir: (dir) async {
          createdDir = dir;
        },
        healthProbe: (_) async {
          probeCount++;
          // Unhealthy before start, then healthy on 2nd poll after start.
          return probeCount >= 3;
        },
        processStarter: (exe, args) async {
          startedExe = exe;
          startedArgs = args;
          return fakeProc;
        },
      );

      await manager.ensureStarted(baseUrl: 'http://127.0.0.1:18899');
      expect(
        startedExe,
        '/Applications/VersionRadar.app/Contents/Resources/versionradar-server',
      );
      expect(createdDir, '/Users/alice/Library/Application Support/VersionRadar/pb_data');
      expect(startedArgs, [
        'serve',
        '--http=127.0.0.1:18899',
        '--dir=/Users/alice/Library/Application Support/VersionRadar/pb_data',
        '--no-open',
      ]);
      expect(manager.isRunning, isTrue);
      expect(manager.lastError, isNull);

      await manager.stop();
      expect(manager.isRunning, isFalse);
      expect(fakeProc.killSignals, [ProcessSignal.sigterm]);
    });

    test('stops sidecar process if health check times out', () async {
      final fakeProc = _FakeProcess();
      final manager = SidecarManager(
        isWeb: false,
        isMacOS: true,
        isWindows: false,
        registerHooks: false,
        resolvedExecutable: () => '/App/Contents/MacOS/VersionRadar',
        currentDirectory: () => '/tmp',
        environment: () => {'HOME': '/Users/alice'},
        fileExists: (p) => p == '/App/Contents/Resources/versionradar-server',
        ensureDir: (_) async {},
        healthProbe: (_) async => false,
        processStarter: (_, _) async => fakeProc,
      );

      await manager.ensureStarted(
        baseUrl: 'http://127.0.0.1:18899',
        healthTimeout: const Duration(milliseconds: 150),
      );
      expect(manager.isRunning, isFalse);
      expect(manager.lastError, contains('Timed out'));
      expect(fakeProc.killSignals, [ProcessSignal.sigterm]);
    });
  });
}
