import 'package:flutter/material.dart';

class AppColors {
  static const bg = Color(0xFF07131F);
  static const navy = Color(0xFF0B1B2B);
  static const card = Color(0xFF0F2235);
  static const cardElevated = Color(0xFF152D45);
  static const border = Color(0xFF1E3A56);
  static const teal = Color(0xFF2DD4BF);
  static const emerald = Color(0xFF34D399);
  static const amber = Color(0xFFFBBF24);
  static const orange = Color(0xFFFB923C);
  static const rose = Color(0xFFFB7185);
  static const sky = Color(0xFF38BDF8);
  static const textPrimary = Color(0xFFEAF2FA);
  static const textMuted = Color(0xFF8AA2B9);
}

ThemeData buildAppTheme() {
  final scheme = ColorScheme.fromSeed(
    seedColor: AppColors.teal,
    brightness: Brightness.dark,
    surface: AppColors.card,
  ).copyWith(
    primary: AppColors.teal,
    onPrimary: AppColors.bg,
    secondary: AppColors.sky,
    surface: AppColors.card,
    surfaceContainerHighest: AppColors.cardElevated,
    outline: AppColors.border,
  );

  return ThemeData(
    useMaterial3: true,
    brightness: Brightness.dark,
    colorScheme: scheme,
    scaffoldBackgroundColor: AppColors.bg,
    appBarTheme: const AppBarTheme(
      backgroundColor: AppColors.bg,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      titleTextStyle: TextStyle(
        color: AppColors.textPrimary,
        fontSize: 19,
        fontWeight: FontWeight.w700,
        letterSpacing: -0.3,
      ),
    ),
    cardTheme: CardThemeData(
      color: AppColors.card,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: const BorderSide(color: AppColors.border, width: 1),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: AppColors.navy,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: const BorderSide(color: AppColors.border, width: 1),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.bg.withValues(alpha: 0.65),
      labelStyle: const TextStyle(color: AppColors.textMuted),
      helperStyle: const TextStyle(color: AppColors.textMuted, fontSize: 12),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.teal, width: 1.5),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: AppColors.teal,
        foregroundColor: AppColors.bg,
        textStyle: const TextStyle(fontWeight: FontWeight.w600),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: AppColors.textPrimary,
        side: const BorderSide(color: AppColors.border),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      ),
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: AppColors.cardElevated,
      contentTextStyle: const TextStyle(color: AppColors.textPrimary),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: AppColors.border),
      ),
      behavior: SnackBarBehavior.floating,
    ),
    dividerTheme: const DividerThemeData(color: AppColors.border, thickness: 1),
  );
}

class ProviderMeta {
  const ProviderMeta(this.label, this.tagline, this.icon, this.color);
  final String label;
  final String tagline;
  final IconData icon;
  final Color color;
}

ProviderMeta providerMeta(String name) => switch (name.toLowerCase()) {
  'cloudflare' => const ProviderMeta(
    'Cloudflare',
    'Pages builds & deployments',
    Icons.cloud_done_outlined,
    Color(0xFFF38020),
  ),
  'heroku' => const ProviderMeta(
    'Heroku',
    'App builds & releases',
    Icons.rocket_launch_outlined,
    Color(0xFFA78BFA),
  ),
  'gcp' => const ProviderMeta(
    'Google Cloud',
    'Cloud Build pipelines',
    Icons.build_circle_outlined,
    Color(0xFF60A5FA),
  ),
  'firebase' => const ProviderMeta(
    'Firebase',
    'App Distribution releases',
    Icons.local_fire_department_outlined,
    Color(0xFFFBBF24),
  ),
  'appstore' => const ProviderMeta(
    'App Store',
    'iOS builds & review status',
    Icons.phone_iphone_outlined,
    Color(0xFF38BDF8),
  ),
  'googleplay' => const ProviderMeta(
    'Google Play',
    'Android release tracks',
    Icons.shop_outlined,
    Color(0xFF34D399),
  ),
  'github' => const ProviderMeta(
    'GitHub',
    'Tags, branches & version files',
    Icons.code_rounded,
    Color(0xFFE2E8F0),
  ),
  'gitlab' => const ProviderMeta(
    'GitLab',
    'Tags, branches & version files',
    Icons.merge_type_rounded,
    Color(0xFFFB923C),
  ),
  'git' => const ProviderMeta(
    'Git (HTTPS)',
    'Any HTTPS repo via Smart HTTP',
    Icons.account_tree_outlined,
    Color(0xFF2DD4BF),
  ),
  'gcp_cloudrun' => const ProviderMeta(
    'Cloud Run',
    'Live revisions & traffic routing',
    Icons.cloud_queue_rounded,
    Color(0xFF38BDF8),
  ),
  'testflight' => const ProviderMeta(
    'TestFlight',
    'iOS beta builds & review',
    Icons.flight_takeoff_rounded,
    Color(0xFF0EA5E9),
  ),
  'slack' => const ProviderMeta(
    'Slack',
    'Channel deployment & drift alerts',
    Icons.tag_rounded,
    Color(0xFFE01E5A),
  ),
  'discord' => const ProviderMeta(
    'Discord',
    'Rich embed deployment alerts',
    Icons.forum_outlined,
    Color(0xFF5865F2),
  ),
  'telegram' => const ProviderMeta(
    'Telegram',
    'Instant bot deployment alerts',
    Icons.send_rounded,
    Color(0xFF26A5E4),
  ),
  _ => ProviderMeta(
    name.isEmpty ? 'Unknown' : name,
    'Custom plugin',
    Icons.extension_outlined,
    AppColors.teal,
  ),
};

String formatRelativeTime(String rawTimestamp) {
  if (rawTimestamp.isEmpty) return '';
  final dt = DateTime.tryParse(rawTimestamp.replaceFirst(' ', 'T'));
  if (dt == null) return rawTimestamp.split('.').first;
  final diff = DateTime.now().toUtc().difference(dt.toUtc());
  if (diff.isNegative || diff.inSeconds < 45) return 'just now';
  if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
  if (diff.inHours < 24) return '${diff.inHours}h ago';
  if (diff.inDays < 30) return '${diff.inDays}d ago';
  return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')}';
}

String shortSha(String s) {
  final t = s.trim();
  return t.length > 7 ? t.substring(0, 7) : t;
}
