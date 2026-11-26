import 'package:flutter/material.dart';
import 'package:pocketbase/pocketbase.dart';

import 'theme.dart';

(IconData, Color) statusStyle(String status) => switch (status) {
  'success' => (Icons.check_circle_rounded, AppColors.emerald),
  'failed' => (Icons.error_rounded, AppColors.rose),
  'in_progress' => (Icons.autorenew_rounded, AppColors.amber),
  _ => (Icons.radio_button_unchecked_rounded, AppColors.textMuted),
};

String statusLabel(String status) => switch (status) {
  'success' => 'Succeeded',
  'failed' => 'Failed',
  'in_progress' => 'In Progress',
  _ => 'Idle',
};

enum RadarHealth { upToDate, outdated, inProgress, failed, synced, idle }

String _normalizeVersion(String v) {
  final t = v.trim();
  if (t.length >= 2 &&
      (t[0] == 'v' || t[0] == 'V') &&
      t.codeUnitAt(1) >= 48 &&
      t.codeUnitAt(1) <= 57) {
    return t.substring(1);
  }
  return t;
}

bool _isHexSha(String s) {
  if (s.length < 7 || s.length > 40) return false;
  return RegExp(r'^[0-9a-fA-F]+$').hasMatch(s);
}

bool _shaPrefixMatch(String a, String b) {
  final la = a.trim().toLowerCase();
  final lb = b.trim().toLowerCase();
  if (!_isHexSha(la) || !_isHexSha(lb)) return false;
  return la.startsWith(lb) || lb.startsWith(la);
}

String _stripBuildMeta(String v) {
  final idx = v.indexOf(RegExp(r'[\s+(]'));
  return idx > 0 ? v.substring(0, idx).trim() : v;
}

/// Mirrors backend core.VersionsMatch so the dashboard immediately indicates whether
/// the deployed version/commit matches the latest version from Git.
bool versionsMatch(
  String deployedVersion,
  String deployedCommit,
  String latestVersion,
  String latestCommit,
) {
  if (latestVersion.isEmpty && latestCommit.isEmpty) return false;
  if (deployedCommit.isNotEmpty &&
      latestCommit.isNotEmpty &&
      _shaPrefixMatch(deployedCommit, latestCommit)) {
    return true;
  }
  if (deployedVersion.isNotEmpty &&
      latestCommit.isNotEmpty &&
      _shaPrefixMatch(deployedVersion, latestCommit)) {
    return true;
  }
  if (deployedCommit.isNotEmpty &&
      latestVersion.isNotEmpty &&
      _shaPrefixMatch(deployedCommit, latestVersion)) {
    return true;
  }
  final dv = _normalizeVersion(deployedVersion);
  final lv = _normalizeVersion(latestVersion);
  if (dv.isEmpty || lv.isEmpty) return false;
  if (dv.toLowerCase() == lv.toLowerCase()) return true;
  final db = _stripBuildMeta(dv);
  final lb = _stripBuildMeta(lv);
  return db.isNotEmpty && db.toLowerCase() == lb.toLowerCase();
}

RadarHealth evaluateRadarHealth(RecordModel radar, RecordModel? latestDep) {
  if (latestDep == null) return RadarHealth.idle;
  final depStatus = latestDep.getStringValue('status');
  if (depStatus == 'in_progress') return RadarHealth.inProgress;
  if (depStatus == 'failed') return RadarHealth.failed;
  if (depStatus == 'success') {
    final latestVer = radar.getStringValue('latest_version');
    final latestCommit = radar.getStringValue('latest_commit');
    if (latestVer.isEmpty && latestCommit.isEmpty) {
      return RadarHealth.synced;
    }
    final matched = versionsMatch(
      latestDep.getStringValue('version'),
      latestDep.getStringValue('commit_sha'),
      latestVer,
      latestCommit,
    );
    return matched ? RadarHealth.upToDate : RadarHealth.outdated;
  }
  return RadarHealth.idle;
}

(String, IconData, Color) healthBadge(RadarHealth h) => switch (h) {
  RadarHealth.upToDate => (
    'Up to date',
    Icons.verified_rounded,
    AppColors.emerald,
  ),
  RadarHealth.outdated => (
    'Outdated',
    Icons.update_rounded,
    AppColors.orange,
  ),
  RadarHealth.inProgress => (
    'Deploying',
    Icons.autorenew_rounded,
    AppColors.amber,
  ),
  RadarHealth.failed => ('Failed', Icons.error_rounded, AppColors.rose),
  RadarHealth.synced => (
    'Healthy',
    Icons.check_circle_rounded,
    AppColors.emerald,
  ),
  RadarHealth.idle => (
    'Waiting',
    Icons.schedule_rounded,
    AppColors.textMuted,
  ),
};
