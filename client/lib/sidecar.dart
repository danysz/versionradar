export 'sidecar_helpers.dart';

import 'sidecar_helpers.dart';
import 'sidecar_stub.dart' if (dart.library.io) 'sidecar_io.dart' as impl;

/// Ensures the bundled `versionradar-server` sidecar is running on desktop
/// platforms (macOS/Windows) and reachable at [baseUrl].
///
/// On web (`kIsWeb`) and non-desktop targets this is a no-op.
Future<void> ensureSidecarStarted({
  String baseUrl = defaultLocalApiUrl,
  Duration healthTimeout = const Duration(seconds: 5),
}) => impl.ensureSidecarStarted(baseUrl: baseUrl, healthTimeout: healthTimeout);

/// Stops the sidecar process started by this app, if any.
Future<void> stopSidecar() => impl.stopSidecar();

/// Whether a sidecar process launched by this app is currently running.
bool get isSidecarRunning => impl.isSidecarRunning;

/// Last non-fatal sidecar startup error message, if any.
String? get sidecarLastError => impl.sidecarLastError;
