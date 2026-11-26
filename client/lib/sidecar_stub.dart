/// Web/non-IO stub for desktop sidecar management.
library;

/// No-op on web and non-IO platforms.
Future<void> ensureSidecarStarted({
  required String baseUrl,
  Duration healthTimeout = const Duration(seconds: 5),
}) async {}

/// No-op on web and non-IO platforms.
Future<void> stopSidecar() async {}

/// Always `false` on web and non-IO platforms.
bool get isSidecarRunning => false;

/// Always `null` on web and non-IO platforms.
String? get sidecarLastError => null;
