import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:pocketbase/pocketbase.dart';

import '../api.dart';
import '../format.dart';
import '../status.dart';
import '../theme.dart';
import '../widgets/modal.dart';
import '../widgets/ui_kit.dart';
import 'webhooks_page.dart';

class DeploymentsPage extends StatefulWidget {
  const DeploymentsPage({super.key, required this.product});
  final RecordModel product;

  @override
  State<DeploymentsPage> createState() => _DeploymentsPageState();
}

class _DeploymentsPageState extends State<DeploymentsPage> {
  late RecordModel _radar = widget.product;
  List<RecordModel> _items = [];
  Object? _error;
  bool _loading = true;
  bool _syncing = false;
  UnsubscribeFunc? _unsubDeps;
  UnsubscribeFunc? _unsubRadar;

  String get _pid => _radar.id;

  @override
  void initState() {
    super.initState();
    _init();
  }

  Future<void> _init() async {
    try {
      final depCol = pb.collection('deployments');
      _unsubDeps = await depCol.subscribe('*', (e) {
        final r = e.record;
        if (r == null || r.getStringValue('product') != _pid) return;
        if (!mounted) return;
        setState(() {
          _items.removeWhere((x) => x.id == r.id);
          if (e.action != 'delete') _items.insert(0, r);
          _items.sort(
            (a, b) => b
                .getStringValue('timestamp')
                .compareTo(a.getStringValue('timestamp')),
          );
        });
      });
      _unsubRadar = await pb.collection('products').subscribe(_pid, (e) {
        if (e.record != null && mounted) {
          setState(() => _radar = e.record!);
        }
      });
      final list = await depCol.getFullList(
        filter: 'product = "$_pid"',
        sort: '-timestamp',
      );
      if (mounted) {
        setState(() {
          _items = list;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e;
          _loading = false;
        });
      }
    }
  }

  @override
  void dispose() {
    _unsubDeps?.call();
    _unsubRadar?.call();
    super.dispose();
  }

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      final res = await syncNow();
      if (!mounted) return;
      showAppToast(context, res.summary, isError: res.problems.isNotEmpty);
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  void _showDeploymentDetails(RecordModel d) {
    final status = d.getStringValue('status');
    final (icon, color) = statusStyle(status);
    final verFmt = DisplayFormat.version(d.getStringValue('version'));
    final sha = d.getStringValue('commit_sha');
    final stage = DisplayFormat.stage(d.getStringValue('stage'));
    final msg = d.getStringValue('message').trim();
    final ts = d.getStringValue('timestamp').split('.').first;
    final extId = d.getStringValue('external_id');

    Widget infoRow(String k, String v) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 96,
            child: Text(
              k,
              style: const TextStyle(
                color: AppColors.textMuted,
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          Expanded(
            child: Text(
              v,
              style: const TextStyle(
                color: AppColors.textPrimary,
                fontSize: 13,
              ),
            ),
          ),
        ],
      ),
    );

    showDialog(
      context: context,
      builder: (c) => AppModal(
        title: verFmt.primary == '—' ? 'Deployment' : 'Version ${verFmt.primary}',
        subtitle: '${statusLabel(status)}${stage.isNotEmpty ? ' · $stage' : ''}',
        icon: icon,
        accentColor: color,
        maxWidth: 480,
        body: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ModalSection(
              title: 'Summary',
              icon: Icons.info_outline_rounded,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  infoRow('Status', statusLabel(status)),
                  if (stage.isNotEmpty) infoRow('Stage', stage),
                  if (verFmt.meta != null) infoRow('Build', verFmt.meta!),
                  if (ts.isNotEmpty)
                    infoRow('Time', '$ts (${formatRelativeTime(ts)})'),
                ],
              ),
            ),
            if (sha.isNotEmpty) ...[
              const SizedBox(height: 12),
              CompactCopyRow(label: 'Commit SHA', value: sha),
            ],
            if (extId.isNotEmpty) ...[
              const SizedBox(height: 10),
              CompactCopyRow(label: 'Provider ID', value: extId),
            ],
            if (msg.isNotEmpty) ...[
              const SizedBox(height: 12),
              ModalSection(
                title: 'Details',
                icon: Icons.notes_rounded,
                child: SelectableText(
                  msg,
                  style: const TextStyle(
                    color: AppColors.textPrimary,
                    fontSize: 13,
                    height: 1.45,
                  ),
                ),
              ),
            ],
          ],
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(c),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  Widget _statBox({
    required String label,
    required String value,
    String? badge,
    String? sub,
    Color valueColor = AppColors.textPrimary,
    Color badgeColor = AppColors.teal,
  }) => Container(
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: AppColors.bg.withValues(alpha: 0.72),
      borderRadius: BorderRadius.circular(14),
      border: Border.all(color: AppColors.border),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label.toUpperCase(),
          style: const TextStyle(
            fontSize: 10.5,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.7,
            color: AppColors.textMuted,
          ),
        ),
        const SizedBox(height: 6),
        Row(
          children: [
            Flexible(
              child: Text(
                value,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontFamily: value != '—' ? 'monospace' : null,
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: valueColor,
                ),
              ),
            ),
            if (badge != null && badge.isNotEmpty) ...[
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                decoration: BoxDecoration(
                  color: badgeColor.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Text(
                  badge,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: badgeColor,
                  ),
                ),
              ),
            ],
          ],
        ),
        if (sub != null && sub.isNotEmpty) ...[
          const SizedBox(height: 4),
          Text(
            sub,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 12, color: AppColors.textMuted),
          ),
        ],
      ],
    ),
  );

  Widget _headerCard() {
    final latestDep = _items.isNotEmpty ? _items.first : null;
    final health = evaluateRadarHealth(_radar, latestDep);
    final (hLabel, hIcon, hColor) = healthBadge(health);
    final plugin = _radar.getStringValue('plugin_name');
    final targetLabel = DisplayFormat.radarTargetLabel(_radar);
    final sourcePlugin = _radar.getStringValue('source_plugin');
    final sourceRepo = DisplayFormat.sourceTargetLabel(_radar);
    final depVerFmt = DisplayFormat.version(
      latestDep?.getStringValue('version') ?? '',
    );
    final gitVerFmt = DisplayFormat.version(
      _radar.getStringValue('latest_version'),
    );
    final gitRefShort = DisplayFormat.shortRef(
      _radar.getStringValue('latest_ref'),
    );
    final sourceErr = _radar.getStringValue('source_error');
    final webhookConnected = _radar.getBoolValue('webhook_connected');

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      ProviderBadge(name: plugin),
                      if (sourcePlugin.isNotEmpty)
                        ProviderBadge(name: sourcePlugin, compact: true),
                      StatusPill(
                        label: hLabel,
                        color: hColor,
                        icon: hIcon,
                        pulsing: health == RadarHealth.inProgress,
                      ),
                    ],
                  ),
                ),
                OutlinedButton.icon(
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute(
                      builder: (_) => WebhooksPage(filterRadarId: _pid),
                    ),
                  ),
                  icon: Icon(
                    webhookConnected
                        ? Icons.bolt_rounded
                        : Icons.link_rounded,
                    size: 16,
                    color: webhookConnected ? AppColors.emerald : AppColors.teal,
                  ),
                  label: Text(webhookConnected ? 'Webhook live' : 'Webhooks'),
                ),
              ],
            ),
            const SizedBox(height: 16),
            LayoutBuilder(
              builder: (_, c) {
                final deployedBox = _statBox(
                  label: 'Deployed Version',
                  value: depVerFmt.primary,
                  badge: depVerFmt.meta,
                  sub: targetLabel.isNotEmpty
                      ? targetLabel
                      : latestDep != null
                      ? formatRelativeTime(
                          latestDep.getStringValue('timestamp'),
                        )
                      : 'No deployments yet',
                  badgeColor: AppColors.sky,
                );
                if (sourcePlugin.isEmpty) return deployedBox;
                final gitBox = _statBox(
                  label: 'Latest in Git',
                  value: sourceErr.isNotEmpty ? 'Error' : gitVerFmt.primary,
                  badge: sourceErr.isNotEmpty ? null : gitRefShort,
                  sub: sourceRepo.isNotEmpty
                      ? sourceRepo
                      : 'Tap Sync to check Git',
                  valueColor: sourceErr.isNotEmpty
                      ? AppColors.rose
                      : health == RadarHealth.upToDate
                      ? AppColors.emerald
                      : health == RadarHealth.outdated
                      ? AppColors.orange
                      : AppColors.sky,
                );
                if (c.maxWidth < 520) {
                  return Column(
                    children: [
                      deployedBox,
                      const SizedBox(height: 10),
                      gitBox,
                    ],
                  );
                }
                return Row(
                  children: [
                    Expanded(child: deployedBox),
                    const SizedBox(width: 12),
                    Expanded(child: gitBox),
                  ],
                );
              },
            ),
            if (sourceErr.isNotEmpty) ...[
              const SizedBox(height: 12),
              ModalBanner(
                message: 'Git check failed: $sourceErr',
                color: AppColors.rose,
                icon: Icons.error_outline_rounded,
              ),
            ],
            if (latestDep != null &&
                latestDep.getStringValue('status') == 'in_progress') ...[
              const SizedBox(height: 14),
              BuildProgressBar(
                progress: latestDep.getIntValue('progress'),
                stage: DisplayFormat.stage(latestDep.getStringValue('stage')),
                color: AppColors.amber,
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _deploymentTile(RecordModel d, bool isLatest) {
    final status = d.getStringValue('status');
    final (icon, color) = statusStyle(status);
    final verFmt = DisplayFormat.version(d.getStringValue('version'));
    final sha = d.getStringValue('commit_sha');
    final stage = DisplayFormat.stage(d.getStringValue('stage'));
    final progress = d.getIntValue('progress');
    final ts = d.getStringValue('timestamp');
    final parsed = DisplayFormat.deploymentMessage(
      d.getStringValue('message'),
      d.getStringValue('platform'),
    );

    return Card(
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => _showDeploymentDetails(d),
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  if (status == 'in_progress')
                    PulsingDot(color: color, size: 11)
                  else
                    Icon(icon, color: color, size: 20),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 6,
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        Text(
                          verFmt.primary,
                          style: const TextStyle(
                            fontFamily: 'monospace',
                            fontWeight: FontWeight.w700,
                            fontSize: 15,
                            color: AppColors.textPrimary,
                          ),
                        ),
                        if (verFmt.meta != null)
                          StatusPill(
                            label: verFmt.meta!,
                            color: AppColors.sky,
                          ),
                        if (isLatest)
                          const StatusPill(
                            label: 'Latest',
                            color: AppColors.teal,
                          ),
                        if (stage.isNotEmpty)
                          StatusPill(label: stage, color: color),
                        if (parsed.env != null)
                          StatusPill(
                            label: parsed.env!,
                            color: AppColors.textMuted,
                          ),
                        if (sha.isNotEmpty && shortSha(sha) != verFmt.primary)
                          InkWell(
                            borderRadius: BorderRadius.circular(6),
                            onTap: () {
                              Clipboard.setData(ClipboardData(text: sha));
                              showAppToast(context, 'Commit SHA copied');
                            },
                            child: Container(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 7,
                                vertical: 2.5,
                              ),
                              decoration: BoxDecoration(
                                color: AppColors.bg,
                                borderRadius: BorderRadius.circular(6),
                                border: Border.all(color: AppColors.border),
                              ),
                              child: Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  const Icon(
                                    Icons.commit_rounded,
                                    size: 12,
                                    color: AppColors.textMuted,
                                  ),
                                  const SizedBox(width: 4),
                                  Text(
                                    shortSha(sha),
                                    style: const TextStyle(
                                      fontFamily: 'monospace',
                                      fontSize: 11.5,
                                      color: AppColors.textMuted,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  Tooltip(
                    message: ts.split('.').first,
                    child: Text(
                      formatRelativeTime(ts),
                      style: const TextStyle(
                        color: AppColors.textMuted,
                        fontSize: 12,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  const Icon(
                    Icons.chevron_right_rounded,
                    size: 18,
                    color: AppColors.textMuted,
                  ),
                ],
              ),
              if (parsed.branch != null || parsed.text.isNotEmpty) ...[
                const SizedBox(height: 8),
                Row(
                  children: [
                    if (parsed.branch != null) ...[
                      const Icon(
                        Icons.call_split_rounded,
                        size: 13,
                        color: AppColors.textMuted,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        parsed.branch!,
                        style: const TextStyle(
                          color: AppColors.textMuted,
                          fontSize: 12.5,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      if (parsed.text.isNotEmpty)
                        const Text(
                          ' · ',
                          style: TextStyle(color: AppColors.textMuted),
                        ),
                    ],
                    if (parsed.text.isNotEmpty)
                      Expanded(
                        child: Text(
                          parsed.text,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            color: AppColors.textMuted,
                            fontSize: 12.5,
                          ),
                        ),
                      ),
                  ],
                ),
              ],
              if (status == 'in_progress') ...[
                const SizedBox(height: 10),
                BuildProgressBar(
                  progress: progress,
                  stage: stage.isEmpty ? 'Building…' : stage,
                  color: color,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(_radar.getStringValue('name')),
      actions: [
        IconButton(
          tooltip: 'Pull latest from provider & Git',
          icon: _syncing
              ? const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.sync_rounded),
          onPressed: _syncing ? null : _sync,
        ),
        const SizedBox(width: 6),
      ],
    ),
    body: _error != null
        ? Center(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: ModalBanner(
                message: DisplayFormat.friendlyError(_error!),
                color: AppColors.rose,
                icon: Icons.error_outline_rounded,
              ),
            ),
          )
        : _loading
        ? const Center(child: CircularProgressIndicator())
        : Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 900),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 32),
                children: [
                  _headerCard(),
                  const SizedBox(height: 20),
                  const Padding(
                    padding: EdgeInsets.only(left: 4, bottom: 10),
                    child: Text(
                      'Deployment Timeline',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                        color: AppColors.textPrimary,
                      ),
                    ),
                  ),
                  if (_items.isEmpty)
                    const Card(
                      child: Padding(
                        padding: EdgeInsets.all(28),
                        child: Center(
                          child: Text(
                            'Waiting for deployments… Tap Sync or connect a webhook.',
                            style: TextStyle(color: AppColors.textMuted),
                          ),
                        ),
                      ),
                    )
                  else
                    for (var i = 0; i < _items.length; i++) ...[
                      _deploymentTile(_items[i], i == 0),
                      const SizedBox(height: 10),
                    ],
                ],
              ),
            ),
          ),
  );
}
