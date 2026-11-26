import 'package:flutter/material.dart';

import 'api.dart';
import 'pages/login_page.dart';
import 'pages/radars_page.dart';
import 'sidecar.dart';
import 'theme.dart';

const brandNavy = AppColors.navy;
const brandTeal = AppColors.teal;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ensureSidecarStarted(baseUrl: apiUrl);
  await initApi();
  runApp(const VersionRadarApp());
}

class VersionRadarApp extends StatelessWidget {
  const VersionRadarApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'VersionRadar',
    debugShowCheckedModeBanner: false,
    theme: buildAppTheme(),
    // Rebuilds on login/logout; the auth store is persisted in api.dart.
    home: StreamBuilder(
      stream: pb.authStore.onChange,
      initialData: null,
      builder: (_, _) =>
          pb.authStore.isValid ? const RadarsPage() : const LoginPage(),
    ),
  );
}
