import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:pocketbase/pocketbase.dart';

import '../api.dart';
import '../format.dart';
import '../status.dart';
import '../theme.dart';
import '../widgets/field_form.dart';
import '../widgets/modal.dart';
import '../widgets/ui_kit.dart';
import 'deployments_page.dart';
import 'plugins_page.dart';
import 'webhooks_page.dart';

/// Home dashboard: live overview of every radar, its deployment status & progress,
/// and a crisp side-by-side comparison of Deployed vs Git Latest versions.
class RadarsPage extends StatefulWidget {
  const RadarsPage({super.key});
  @override
  State<RadarsPage> createState() => _RadarsPageState();
}

class _RadarsPageState extends State<RadarsPage> {
  List<RecordModel> _radars = [];
  Set<String> _enabledPlugins = {};
  final Map<String, RecordModel> _latest = {}; // radar id -> newest deployment
  Object? _error;
  bool _loading = true;
  bool _syncing = false;
  String _search = '';
  RadarHealth? _healthFilter;
  UnsubscribeFunc? _unsubDeps;
  UnsubscribeFunc? _unsubRadars;

  @override
  void initState() {
    super.initState();
    _init();
  }

  @override
  void dispose() {
    _unsubDeps?.call();
    _unsubRadars?.call();
    super.dispose();
  }

  void _trackDep(RecordModel d) {
    final id = d.getStringValue('product');
    final cur = _latest[id];
    if (cur == null ||
        d
                .getStringValue('timestamp')
                .compareTo(cur.getStringValue('timestamp')) >=
            0) {
      _latest[id] = d;
    }
  }

  Future<void> _init() async {
    try {
      _unsubDeps = await pb.collection('deployments').subscribe('*', (e) {
        if (e.record != null && e.action != 'delete' && mounted) {
          setState(() => _trackDep(e.record!));
        }
      });
      _unsubRadars = await pb.collection('products').subscribe('*', (e) {
        final r = e.record;
        if (r == null || !mounted) return;
        setState(() {
          if (e.action == 'delete') {
            _radars.removeWhere((x) => x.id == r.id);
            _latest.remove(r.id);
          } else {
            final idx = _radars.indexWhere((x) => x.id == r.id);
            if (idx >= 0) {
              _radars[idx] = r;
            } else {
              _radars.add(r);
            }
            _radars.sort(
              (a, b) => a
                  .getStringValue('name')
                  .toLowerCase()
                  .compareTo(b.getStringValue('name').toLowerCase()),
            );
          }
        });
      });
      await _load();
    } catch (e) {
      if (mounted) setState(() => _error = e);
    }
  }

  Future<void> _load() async {
    final radars = await pb.collection('products').getFullList(sort: 'name');
    final plugins = await pb
        .collection('user_plugins')
        .getFullList(filter: 'is_enabled = true');
    final deps = await pb
        .collection('deployments')
        .getList(perPage: 200, sort: '-timestamp');
    if (!mounted) return;
    setState(() {
      _radars = radars;
      _enabledPlugins = {
        for (final p in plugins) p.getStringValue('plugin_name'),
      };
      for (final d in deps.items) {
        _trackDep(d);
      }
      _loading = false;
    });
  }

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      final res = await syncNow();
      if (!mounted) return;
      if (res.problems.isEmpty) {
        showAppToast(context, res.summary);
      } else {
        showAppToast(
          context,
          res.summary,
          isError: true,
          actionLabel: res.problems.length > 1 ? 'Details' : null,
          onAction: () => _showSyncIssuesModal(res.problems),
        );
      }
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  void _showSyncIssuesModal(List<String> problems) {
    showDialog(
      context: context,
      builder: (c) => AppModal(
        title: 'Sync Issues',
        subtitle: '${problems.length} provider(s) reported an issue',
        icon: Icons.warning_amber_rounded,
        accentColor: AppColors.amber,
        body: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (var i = 0; i < problems.length; i++) ...[
              if (i > 0) const SizedBox(height: 10),
              ModalBanner(
                message: problems[i],
                color: AppColors.rose,
                icon: Icons.error_outline_rounded,
              ),
            ],
          ],
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(c),
            child: const Text('Got it'),
          ),
        ],
      ),
    );
  }

  Map<String, dynamic> _parseJsonMap(String raw) {
    if (raw.trim().isEmpty) return {};
    try {
      final decoded = jsonDecode(raw);
      if (decoded is Map) return decoded.cast<String, dynamic>();
    } catch (_) {}
    return {};
  }

  Future<void> _openRadarDialog({RecordModel? existing}) async {
    try {
      final catalog = await fetchPluginsCatalog();
      final providers = ((catalog['plugins'] as List?) ?? const [])
          .cast<Map<String, dynamic>>();
      final sources = ((catalog['sources'] as List?) ?? const [])
          .cast<Map<String, dynamic>>();
      final strategies = ((catalog['version_strategies'] as List?) ?? const [])
          .cast<Map<String, dynamic>>();
      if (!mounted || providers.isEmpty) return;

      final initTarget = existing != null
          ? _parseJsonMap(existing.getStringValue('target'))
          : <String, dynamic>{};
      final initSrcTarget = existing != null
          ? _parseJsonMap(existing.getStringValue('source_target'))
          : <String, dynamic>{};
      final initVerCfg = existing != null
          ? _parseJsonMap(existing.getStringValue('version_config'))
          : <String, dynamic>{};

      final nameCtrl = TextEditingController(
        text: existing?.getStringValue('name') ?? '',
      );
      var provider = providers.firstWhere(
        (p) => p['name'] == existing?.getStringValue('plugin_name'),
        orElse: () => providers.first,
      );
      var targetForm = FieldForm(
        provider['target_fields'] as List?,
        initialValues: initTarget,
      );

      var enableGit =
          existing != null && existing.getStringValue('source_plugin').isNotEmpty;
      var source = sources.firstWhere(
        (s) => s['name'] == existing?.getStringValue('source_plugin'),
        orElse: () => sources.first,
      );
      var sourceForm = FieldForm(
        source['target_fields'] as List?,
        initialValues: initSrcTarget,
      );
      var strategy = strategies.firstWhere(
        (s) => s['name'] == existing?.getStringValue('version_strategy'),
        orElse: () => strategies.first,
      );
      var strategyForm = FieldForm(
        strategy['config_fields'] as List?,
        initialValues: initVerCfg,
      );
      String? error;

      final ok = await showDialog<bool>(
        context: context,
        builder: (c) => StatefulBuilder(
          builder: (c, set) {
            final provMeta = providerMeta(provider['name'] as String);
            return AppModal(
              title: existing == null ? 'New Radar' : 'Edit Radar',
              subtitle: existing == null
                  ? 'Watch a deployment target and compare against Git'
                  : 'Update deployment target or Git version check',
              icon: Icons.radar_rounded,
              accentColor: provMeta.color,
              maxWidth: 540,
              body: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  ModalSection(
                    title: 'General',
                    icon: Icons.label_outline_rounded,
                    child: TextField(
                      controller: nameCtrl,
                      autofocus: existing == null,
                      decoration: const InputDecoration(
                        labelText: 'Radar Name',
                        hintText: 'e.g. Production API, iOS App, Web',
                      ),
                    ),
                  ),
                  const SizedBox(height: 14),
                  ModalSection(
                    title: 'Deployment Target',
                    icon: provMeta.icon,
                    trailing: ProviderBadge(
                      name: provider['name'] as String,
                      compact: true,
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        DropdownButtonFormField<String>(
                          initialValue: provider['name'] as String,
                          decoration: const InputDecoration(
                            labelText: 'Provider',
                          ),
                          items: [
                            for (final p in providers)
                              DropdownMenuItem(
                                value: p['name'] as String,
                                child: Row(
                                  children: [
                                    Icon(
                                      providerMeta(p['name'] as String).icon,
                                      size: 16,
                                      color: providerMeta(
                                        p['name'] as String,
                                      ).color,
                                    ),
                                    const SizedBox(width: 8),
                                    Text(
                                      providerMeta(p['name'] as String).label,
                                    ),
                                  ],
                                ),
                              ),
                          ],
                          onChanged: (v) => set(() {
                            provider = providers.firstWhere(
                              (p) => p['name'] == v,
                            );
                            targetForm = FieldForm(
                              provider['target_fields'] as List?,
                            );
                          }),
                        ),
                        ...targetForm.inputs(),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),
                  ModalSection(
                    title: 'Git Version Comparison',
                    icon: Icons.account_tree_outlined,
                    trailing: Switch(
                      value: enableGit,
                      onChanged: (v) => set(() => enableGit = v),
                    ),
                    child: !enableGit
                        ? const Text(
                            'Optional: link a Git repo to highlight when the deployed version is behind your latest tag, branch commit, or version file.',
                            style: TextStyle(
                              color: AppColors.textMuted,
                              fontSize: 12.5,
                              height: 1.4,
                            ),
                          )
                        : Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              if (sources.isNotEmpty) ...[
                                DropdownButtonFormField<String>(
                                  initialValue: source['name'] as String,
                                  decoration: const InputDecoration(
                                    labelText: 'Git Host',
                                  ),
                                  items: [
                                    for (final s in sources)
                                      DropdownMenuItem(
                                        value: s['name'] as String,
                                        child: Row(
                                          children: [
                                            Icon(
                                              providerMeta(
                                                s['name'] as String,
                                              ).icon,
                                              size: 16,
                                              color: providerMeta(
                                                s['name'] as String,
                                              ).color,
                                            ),
                                            const SizedBox(width: 8),
                                            Text(
                                              providerMeta(
                                                s['name'] as String,
                                              ).label,
                                            ),
                                          ],
                                        ),
                                      ),
                                  ],
                                  onChanged: (v) => set(() {
                                    source = sources.firstWhere(
                                      (s) => s['name'] == v,
                                    );
                                    sourceForm = FieldForm(
                                      source['target_fields'] as List?,
                                    );
                                  }),
                                ),
                                ...sourceForm.inputs(),
                              ],
                              if (strategies.isNotEmpty) ...[
                                const SizedBox(height: 12),
                                DropdownButtonFormField<String>(
                                  initialValue: strategy['name'] as String,
                                  decoration: InputDecoration(
                                    labelText: 'How to read version',
                                    helperText:
                                        strategy['description'] as String?,
                                    helperMaxLines: 2,
                                  ),
                                  items: [
                                    for (final st in strategies)
                                      DropdownMenuItem(
                                        value: st['name'] as String,
                                        child: Text(st['label'] as String),
                                      ),
                                  ],
                                  onChanged: (v) => set(() {
                                    strategy = strategies.firstWhere(
                                      (st) => st['name'] == v,
                                    );
                                    strategyForm = FieldForm(
                                      strategy['config_fields'] as List?,
                                    );
                                  }),
                                ),
                                ...strategyForm.inputs(),
                              ],
                            ],
                          ),
                  ),
                  if (error != null) ...[
                    const SizedBox(height: 14),
                    ModalBanner(
                      message: error!,
                      color: AppColors.rose,
                      icon: Icons.error_outline_rounded,
                    ),
                  ],
                ],
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.pop(c, false),
                  child: const Text('Cancel'),
                ),
                FilledButton.icon(
                  icon: Icon(
                    existing == null ? Icons.add_rounded : Icons.check_rounded,
                    size: 18,
                  ),
                  onPressed: () {
                    if (nameCtrl.text.trim().isEmpty) {
                      set(() => error = 'Radar Name is required');
                      return;
                    }
                    final (_, tErr) = targetForm.values();
                    if (tErr != null) {
                      set(() => error = tErr);
                      return;
                    }
                    if (enableGit) {
                      final (_, sErr) = sourceForm.values();
                      if (sErr != null) {
                        set(() => error = sErr);
                        return;
                      }
                      final (_, vErr) = strategyForm.values();
                      if (vErr != null) {
                        set(() => error = vErr);
                        return;
                      }
                    }
                    Navigator.pop(c, true);
                  },
                  label: Text(existing == null ? 'Create Radar' : 'Save'),
                ),
              ],
            );
          },
        ),
      );
      if (ok != true) return;

      final body = <String, dynamic>{
        'name': nameCtrl.text.trim(),
        'plugin_name': provider['name'],
        'target': jsonEncode(targetForm.values().$1),
        'source_plugin': enableGit ? source['name'] : '',
        'source_target': enableGit ? jsonEncode(sourceForm.values().$1) : '',
        'version_strategy': enableGit ? strategy['name'] : '',
        'version_config': enableGit
            ? jsonEncode(strategyForm.values().$1)
            : '',
      };

      if (existing == null) {
        body['user'] = pb.authStore.record!.id;
        await pb.collection('products').create(body: body);
      } else {
        await pb.collection('products').update(existing.id, body: body);
      }
      await _load();
      _sync();
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    }
  }

  Future<void> _delete(RecordModel r) async {
    final name = r.getStringValue('name');
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AppModal(
        title: 'Delete Radar',
        subtitle: name,
        icon: Icons.delete_outline_rounded,
        accentColor: AppColors.rose,
        maxWidth: 420,
        body: const ModalBanner(
          message:
              'This removes the radar and its stored deployment history from VersionRadar. Nothing is changed at your cloud provider.',
          color: AppColors.rose,
          icon: Icons.warning_amber_rounded,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(c, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: AppColors.rose),
            onPressed: () => Navigator.pop(c, true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (ok == true) {
      await pb.collection('products').delete(r.id);
      _latest.remove(r.id);
      await _load();
      if (mounted) showAppToast(context, 'Deleted "$name"');
    }
  }

  List<RecordModel> get _filteredRadars {
    final q = _search.trim().toLowerCase();
    return _radars.where((r) {
      if (_healthFilter != null &&
          evaluateRadarHealth(r, _latest[r.id]) != _healthFilter) {
        return false;
      }
      if (q.isEmpty) return true;
      final hay = [
        r.getStringValue('name'),
        r.getStringValue('plugin_name'),
        r.getStringValue('source_plugin'),
        DisplayFormat.radarTargetLabel(r),
        r.getStringValue('latest_version'),
        _latest[r.id]?.getStringValue('version') ?? '',
      ].join(' ').toLowerCase();
      return hay.contains(q);
    }).toList();
  }

  Widget _kpiStrip() {
    var upToDate = 0, outdated = 0, inProgress = 0, failed = 0;
    for (final r in _radars) {
      switch (evaluateRadarHealth(r, _latest[r.id])) {
        case RadarHealth.upToDate:
        case RadarHealth.synced:
          upToDate++;
        case RadarHealth.outdated:
          outdated++;
        case RadarHealth.inProgress:
          inProgress++;
        case RadarHealth.failed:
          failed++;
        case RadarHealth.idle:
          break;
      }
    }

    Widget tile(
      String label,
      int count,
      Color color,
      IconData icon,
      RadarHealth? filter,
    ) {
      final selected = _healthFilter == filter && filter != null;
      return InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: () => setState(
          () => _healthFilter = (_healthFilter == filter) ? null : filter,
        ),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          decoration: BoxDecoration(
            color: selected ? color.withValues(alpha: 0.16) : AppColors.card,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(
              color: selected ? color : AppColors.border,
              width: selected ? 1.5 : 1,
            ),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(icon, size: 15, color: color),
                  const SizedBox(width: 6),
                  Text(
                    label,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: AppColors.textMuted,
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 6),
              Text(
                '$count',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: count > 0 ? color : AppColors.textPrimary,
                ),
              ),
            ],
          ),
        ),
      );
    }

    final tiles = [
      tile('All', _radars.length, AppColors.teal, Icons.radar_rounded, null),
      tile(
        'Latest',
        upToDate,
        AppColors.emerald,
        Icons.verified_rounded,
        RadarHealth.upToDate,
      ),
      tile(
        'Outdated',
        outdated,
        AppColors.orange,
        Icons.update_rounded,
        RadarHealth.outdated,
      ),
      tile(
        'Building',
        inProgress,
        AppColors.amber,
        Icons.autorenew_rounded,
        RadarHealth.inProgress,
      ),
      tile(
        'Failed',
        failed,
        AppColors.rose,
        Icons.error_rounded,
        RadarHealth.failed,
      ),
    ];

    return LayoutBuilder(
      builder: (_, c) {
        if (c.maxWidth < 560) {
          return SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: [
                for (var i = 0; i < tiles.length; i++) ...[
                  if (i > 0) const SizedBox(width: 8),
                  SizedBox(width: 112, child: tiles[i]),
                ],
              ],
            ),
          );
        }
        return Row(
          children: [
            for (var i = 0; i < tiles.length; i++) ...[
              if (i > 0) const SizedBox(width: 10),
              Expanded(child: tiles[i]),
            ],
          ],
        );
      },
    );
  }

  Widget _metricColumn({
    required String label,
    required String value,
    String? badge,
    Color valueColor = AppColors.textPrimary,
    Color badgeColor = AppColors.textMuted,
  }) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        label.toUpperCase(),
        style: const TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.7,
          color: AppColors.textMuted,
        ),
      ),
      const SizedBox(height: 4),
      Row(
        children: [
          Flexible(
            child: Text(
              value,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontFamily: value != '—' ? 'monospace' : null,
                fontSize: 14,
                fontWeight: FontWeight.w700,
                color: valueColor,
              ),
            ),
          ),
          if (badge != null && badge.isNotEmpty) ...[
            const SizedBox(width: 6),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1.5),
              decoration: BoxDecoration(
                color: badgeColor.withValues(alpha: 0.14),
                borderRadius: BorderRadius.circular(5),
              ),
              child: Text(
                badge,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 10.5,
                  fontWeight: FontWeight.w600,
                  color: badgeColor,
                ),
              ),
            ),
          ],
        ],
      ),
    ],
  );

  Widget _radarCard(RecordModel r) {
    final d = _latest[r.id];
    final plugin = r.getStringValue('plugin_name');
    final sourcePlugin = r.getStringValue('source_plugin');
    final targetLabel = DisplayFormat.radarTargetLabel(r);
    final webhookLive = r.getBoolValue('webhook_connected');
    final needsCreds = plugin.isNotEmpty && !_enabledPlugins.contains(plugin);

    final health = evaluateRadarHealth(r, d);
    final (hLabel, hIcon, hColor) = healthBadge(health);

    final depVerFmt = DisplayFormat.version(d?.getStringValue('version') ?? '');
    final gitVerFmt = DisplayFormat.version(
      r.getStringValue('latest_version'),
    );
    final gitRefShort = DisplayFormat.shortRef(r.getStringValue('latest_ref'));
    final sourceErr = r.getStringValue('source_error');

    final depStage = DisplayFormat.stage(d?.getStringValue('stage') ?? '');
    final depProg = d?.getIntValue('progress') ?? 0;
    final depTs = d?.getStringValue('timestamp') ?? '';
    final parsedMsg = DisplayFormat.deploymentMessage(
      d?.getStringValue('message') ?? '',
      plugin,
    );

    return Card(
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => Navigator.push(
          context,
          MaterialPageRoute(builder: (_) => DeploymentsPage(product: r)),
        ),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Top row: Name + Target subtitle + Health pill + Menu
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          r.getStringValue('name'),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
                            color: AppColors.textPrimary,
                          ),
                        ),
                        if (targetLabel.isNotEmpty) ...[
                          const SizedBox(height: 2),
                          Text(
                            targetLabel,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 12,
                              color: AppColors.textMuted,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  StatusPill(
                    label: hLabel,
                    color: hColor,
                    icon: hIcon,
                    pulsing: health == RadarHealth.inProgress,
                  ),
                  SizedBox(
                    width: 28,
                    height: 28,
                    child: PopupMenuButton<String>(
                      padding: EdgeInsets.zero,
                      icon: const Icon(
                        Icons.more_vert_rounded,
                        color: AppColors.textMuted,
                        size: 18,
                      ),
                      onSelected: (act) {
                        switch (act) {
                          case 'edit':
                            _openRadarDialog(existing: r);
                          case 'webhook':
                            Navigator.push(
                              context,
                              MaterialPageRoute(
                                builder: (_) =>
                                    WebhooksPage(filterRadarId: r.id),
                              ),
                            );
                          case 'delete':
                            _delete(r);
                        }
                      },
                      itemBuilder: (_) => const [
                        PopupMenuItem(
                          value: 'edit',
                          child: ListTile(
                            dense: true,
                            contentPadding: EdgeInsets.zero,
                            leading: Icon(Icons.edit_outlined, size: 18),
                            title: Text('Edit radar'),
                          ),
                        ),
                        PopupMenuItem(
                          value: 'webhook',
                          child: ListTile(
                            dense: true,
                            contentPadding: EdgeInsets.zero,
                            leading: Icon(Icons.bolt_rounded, size: 18),
                            title: Text('Webhooks'),
                          ),
                        ),
                        PopupMenuItem(
                          value: 'delete',
                          child: ListTile(
                            dense: true,
                            contentPadding: EdgeInsets.zero,
                            leading: Icon(
                              Icons.delete_outline_rounded,
                              size: 18,
                              color: AppColors.rose,
                            ),
                            title: Text(
                              'Delete',
                              style: TextStyle(color: AppColors.rose),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 10),
              // Compact badges row
              Wrap(
                spacing: 6,
                runSpacing: 4,
                children: [
                  ProviderBadge(name: plugin, compact: true),
                  if (sourcePlugin.isNotEmpty)
                    ProviderBadge(name: sourcePlugin, compact: true),
                  if (webhookLive)
                    const StatusPill(
                      label: 'Live',
                      color: AppColors.teal,
                      icon: Icons.bolt_rounded,
                    ),
                ],
              ),
              const SizedBox(height: 12),
              // Clean 2-column version tile: Deployed vs Git Latest
              Container(
                width: double.infinity,
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: AppColors.bg.withValues(alpha: 0.72),
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(
                    color: health == RadarHealth.outdated
                        ? AppColors.orange.withValues(alpha: 0.45)
                        : AppColors.border,
                  ),
                ),
                child: Row(
                  children: [
                    Expanded(
                      child: _metricColumn(
                        label: 'Deployed',
                        value: needsCreds
                            ? 'Setup needed'
                            : depVerFmt.primary,
                        badge: depVerFmt.meta ?? parsedMsg.env,
                        valueColor: needsCreds
                            ? AppColors.amber
                            : AppColors.textPrimary,
                        badgeColor: AppColors.sky,
                      ),
                    ),
                    if (sourcePlugin.isNotEmpty) ...[
                      Container(
                        width: 1,
                        height: 30,
                        margin: const EdgeInsets.symmetric(horizontal: 12),
                        color: AppColors.border,
                      ),
                      Expanded(
                        child: _metricColumn(
                          label: 'Git Latest',
                          value: sourceErr.isNotEmpty
                              ? 'Error'
                              : gitVerFmt.primary,
                          badge: sourceErr.isNotEmpty ? null : gitRefShort,
                          valueColor: sourceErr.isNotEmpty
                              ? AppColors.rose
                              : health == RadarHealth.upToDate
                              ? AppColors.emerald
                              : health == RadarHealth.outdated
                              ? AppColors.orange
                              : AppColors.sky,
                          badgeColor: AppColors.teal,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const Spacer(),
              // Bottom strip: live progress OR clean 1-line footer with timestamp
              if (health == RadarHealth.inProgress)
                BuildProgressBar(
                  progress: depProg,
                  stage: depStage.isEmpty ? 'Building…' : depStage,
                  color: AppColors.amber,
                )
              else
                Row(
                  children: [
                    if (parsedMsg.branch != null) ...[
                      Icon(
                        Icons.call_split_rounded,
                        size: 13,
                        color: AppColors.textMuted,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        parsedMsg.branch!,
                        style: const TextStyle(
                          fontSize: 11.5,
                          color: AppColors.textMuted,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      if (parsedMsg.text.isNotEmpty)
                        const Text(
                          ' · ',
                          style: TextStyle(color: AppColors.textMuted),
                        ),
                    ],
                    Expanded(
                      child: Text(
                        parsedMsg.text.isNotEmpty
                            ? parsedMsg.text
                            : depStage.isNotEmpty
                            ? depStage
                            : needsCreds
                            ? 'Add credentials in Plugins'
                            : 'Waiting for first deployment',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 12,
                          color: AppColors.textMuted,
                        ),
                      ),
                    ),
                    if (depTs.isNotEmpty) ...[
                      const SizedBox(width: 8),
                      Text(
                        formatRelativeTime(depTs),
                        style: const TextStyle(
                          fontSize: 11.5,
                          color: AppColors.textMuted,
                        ),
                      ),
                    ],
                  ],
                ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final visible = _filteredRadars;
    return Scaffold(
      appBar: AppBar(
        title: Row(
          children: [
            Image.asset('assets/logo.png', height: 28),
            const SizedBox(width: 10),
            const Text('VersionRadar'),
          ],
        ),
        actions: [
          IconButton(
            tooltip: 'Sync providers & Git now',
            icon: _syncing
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.sync_rounded),
            onPressed: _syncing ? null : _sync,
          ),
          IconButton(
            tooltip: 'Webhook Automation',
            icon: const Icon(Icons.bolt_rounded),
            onPressed: () async {
              await Navigator.push(
                context,
                MaterialPageRoute(builder: (_) => const WebhooksPage()),
              );
              _load();
            },
          ),
          IconButton(
            tooltip: 'Plugins & Git Sources',
            icon: const Icon(Icons.extension_rounded),
            onPressed: () async {
              await Navigator.push(
                context,
                MaterialPageRoute(builder: (_) => const PluginsPage()),
              );
              _load();
            },
          ),
          IconButton(
            tooltip: 'Sign out',
            icon: const Icon(Icons.logout_rounded),
            onPressed: pb.authStore.clear,
          ),
          const SizedBox(width: 4),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        backgroundColor: AppColors.teal,
        foregroundColor: AppColors.bg,
        onPressed: () => _openRadarDialog(),
        icon: const Icon(Icons.add_rounded),
        label: const Text(
          'Add Radar',
          style: TextStyle(fontWeight: FontWeight.w700),
        ),
      ),
      body: _error != null
          ? Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: ModalBanner(
                  message: DisplayFormat.friendlyError(_error!),
                  color: AppColors.rose,
                  icon: Icons.cloud_off_rounded,
                ),
              ),
            )
          : _loading
          ? const Center(child: CircularProgressIndicator())
          : _radars.isEmpty
          ? Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Image.asset('assets/logo.png', height: 72),
                    const SizedBox(height: 16),
                    const Text(
                      'No radars yet',
                      style: TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Add your first radar to track deployments and compare them against Git.',
                      textAlign: TextAlign.center,
                      style: TextStyle(color: AppColors.textMuted),
                    ),
                    const SizedBox(height: 20),
                    FilledButton.icon(
                      onPressed: () => _openRadarDialog(),
                      icon: const Icon(Icons.add_rounded),
                      label: const Text('Add Radar'),
                    ),
                  ],
                ),
              ),
            )
          : Center(
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 1240),
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final cols = constraints.maxWidth >= 1020
                        ? 3
                        : constraints.maxWidth >= 660
                        ? 2
                        : 1;
                    return CustomScrollView(
                      slivers: [
                        SliverPadding(
                          padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
                          sliver: SliverToBoxAdapter(
                            child: Column(
                              children: [
                                _kpiStrip(),
                                const SizedBox(height: 12),
                                TextField(
                                  onChanged: (v) => setState(() => _search = v),
                                  decoration: InputDecoration(
                                    hintText:
                                        'Search radars, providers, versions…',
                                    prefixIcon: const Icon(
                                      Icons.search_rounded,
                                      size: 20,
                                    ),
                                    suffixIcon: _healthFilter != null
                                        ? TextButton(
                                            onPressed: () => setState(
                                              () => _healthFilter = null,
                                            ),
                                            child: const Text('Clear filter'),
                                          )
                                        : null,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                        if (visible.isEmpty)
                          const SliverFillRemaining(
                            hasScrollBody: false,
                            child: Center(
                              child: Text(
                                'No radars match this filter.',
                                style: TextStyle(color: AppColors.textMuted),
                              ),
                            ),
                          )
                        else if (cols == 1)
                          SliverPadding(
                            padding: const EdgeInsets.fromLTRB(16, 0, 16, 96),
                            sliver: SliverList.separated(
                              itemCount: visible.length,
                              separatorBuilder: (_, _) =>
                                  const SizedBox(height: 12),
                              itemBuilder: (_, i) => SizedBox(
                                height: 224,
                                child: _radarCard(visible[i]),
                              ),
                            ),
                          )
                        else
                          SliverPadding(
                            padding: const EdgeInsets.fromLTRB(16, 0, 16, 96),
                            sliver: SliverGrid(
                              gridDelegate:
                                  SliverGridDelegateWithFixedCrossAxisCount(
                                    crossAxisCount: cols,
                                    mainAxisSpacing: 12,
                                    crossAxisSpacing: 12,
                                    mainAxisExtent: 224,
                                  ),
                              delegate: SliverChildBuilderDelegate(
                                (_, i) => _radarCard(visible[i]),
                                childCount: visible.length,
                              ),
                            ),
                          ),
                      ],
                    );
                  },
                ),
              ),
            ),
    );
  }
}
