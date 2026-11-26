import 'package:flutter/material.dart';

import '../api.dart';
import '../format.dart';
import '../theme.dart';
import '../widgets/modal.dart';
import '../widgets/ui_kit.dart';

/// Clean webhook automation center: concise per-radar status cards, 1-click Auto-Connect,
/// formatted Manual/CLI setup modal, and a combined fallback bash script tab.
class WebhooksPage extends StatefulWidget {
  const WebhooksPage({super.key, this.filterRadarId});
  final String? filterRadarId;

  @override
  State<WebhooksPage> createState() => _WebhooksPageState();
}

class _WebhooksPageState extends State<WebhooksPage>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs = TabController(length: 2, vsync: this);
  List<Map<String, dynamic>> _plans = [];
  String _script = '';
  bool _loading = true;
  bool _busyAll = false;
  final Set<String> _busyKeys = {};
  Object? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  String _planKey(Map<String, dynamic> p) =>
      '${p['radar_id']}:${p['kind']}:${p['plugin']}';

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final res = await fetchWebhookPlans();
      final all = ((res['plans'] as List?) ?? const [])
          .cast<Map<String, dynamic>>();
      final filtered = widget.filterRadarId == null
          ? all
          : all.where((p) => p['radar_id'] == widget.filterRadarId).toList();
      if (!mounted) return;
      setState(() {
        _plans = filtered;
        _script = (res['script'] as String?) ?? '';
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _connectAll() async {
    setState(() => _busyAll = true);
    try {
      final res = await connectAllWebhooks();
      final results = ((res['results'] as List?) ?? const [])
          .cast<Map<String, dynamic>>();
      final filtered = widget.filterRadarId == null
          ? results
          : results
              .where((p) => p['radar_id'] == widget.filterRadarId)
              .toList();
      final failed = filtered.where((r) => (r['error'] ?? '') != '').length;
      if (!mounted) return;
      setState(() {
        _plans = filtered;
        _script = (res['script'] as String?) ?? _script;
      });
      showAppToast(
        context,
        failed == 0
            ? 'Connected ${res['connected']} webhook(s)'
            : 'Connected ${res['connected']} · $failed need manual/CLI setup',
        isError: failed > 0,
      );
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    } finally {
      if (mounted) setState(() => _busyAll = false);
    }
  }

  Future<void> _connectOne(
    Map<String, dynamic> plan, {
    bool askOverride = false,
  }) async {
    Map<String, dynamic>? overrideCfg;
    if (askOverride) {
      overrideCfg = await _promptOverrideToken(plan['plugin'] as String);
      if (overrideCfg == null) return;
    }
    final key = _planKey(plan);
    setState(() => _busyKeys.add(key));
    try {
      final updated = await connectRadarWebhook(
        plan['radar_id'] as String,
        kind: (plan['kind'] as String?) ?? 'provider',
        overrideConfig: overrideCfg,
      );
      if (!mounted) return;
      setState(() {
        final idx = _plans.indexWhere((p) => _planKey(p) == key);
        if (idx >= 0) _plans[idx] = updated;
      });
      final err = updated['error'] as String?;
      if (err != null && err.isNotEmpty) {
        showAppToast(
          context,
          'Auto-connect failed — use Setup Guide fallback',
          isError: true,
          actionLabel: 'Guide',
          onAction: () => _showFallbackModal(updated),
        );
      } else {
        showAppToast(
          context,
          'Connected webhook for ${plan['radar_name']}',
        );
      }
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    } finally {
      if (mounted) setState(() => _busyKeys.remove(key));
    }
  }

  Future<Map<String, dynamic>?> _promptOverrideToken(String plugin) async {
    final ctrl = TextEditingController();
    final meta = providerMeta(plugin);
    final fieldKey = switch (plugin) {
      'github' || 'gitlab' => 'token',
      'gcp' => 'service_account_json',
      _ => 'api_token',
    };
    final label = switch (plugin) {
      'gcp' => 'Service Account JSON (Pub/Sub Editor)',
      'github' => 'GitHub Token (Webhooks: Write)',
      'gitlab' => 'GitLab Token (api scope)',
      'cloudflare' => 'Cloudflare Token (Notifications: Edit)',
      _ => 'Elevated API Token',
    };
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AppModal(
        title: 'One-Time Setup Token',
        subtitle: meta.label,
        icon: Icons.key_rounded,
        accentColor: meta.color,
        maxWidth: 460,
        body: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const ModalBanner(
              message:
                  'Used once to create the webhook and immediately discarded. Your saved polling credentials remain read-only.',
              color: AppColors.teal,
              icon: Icons.shield_outlined,
            ),
            const SizedBox(height: 14),
            TextField(
              controller: ctrl,
              obscureText: plugin != 'gcp',
              minLines: plugin == 'gcp' ? 4 : 1,
              maxLines: plugin == 'gcp' ? 6 : 1,
              decoration: InputDecoration(labelText: label),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(c, false),
            child: const Text('Cancel'),
          ),
          FilledButton.icon(
            icon: const Icon(Icons.bolt_rounded, size: 17),
            onPressed: () => Navigator.pop(c, true),
            label: const Text('Connect'),
          ),
        ],
      ),
    );
    if (ok != true || ctrl.text.trim().isEmpty) return null;
    return {fieldKey: ctrl.text.trim()};
  }

  void _showFallbackModal(Map<String, dynamic> p) {
    final plugin = (p['plugin'] as String?) ?? '';
    final meta = providerMeta(plugin);
    final url = (p['webhook_url'] as String?) ?? '';
    final cli = (p['cli_command'] as String?) ?? '';
    final manual = (p['manual_steps'] as String?) ?? '';

    showDialog(
      context: context,
      builder: (c) => AppModal(
        title: 'Manual & CLI Setup',
        subtitle: '${p['radar_name'] ?? 'Radar'} · ${meta.label}',
        icon: Icons.terminal_rounded,
        accentColor: meta.color,
        maxWidth: 560,
        body: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (url.isNotEmpty) ...[
              ModalSection(
                title: 'Webhook Endpoint',
                icon: Icons.link_rounded,
                child: CompactCopyRow(label: 'URL', value: url),
              ),
              const SizedBox(height: 14),
            ],
            if (cli.isNotEmpty) ...[
              ModalSection(
                title: 'Run via CLI',
                icon: Icons.terminal_rounded,
                child: CopyableCodeBlock(
                  title: 'Shell command',
                  text: cli,
                  maxHeight: 150,
                ),
              ),
              const SizedBox(height: 14),
            ],
            if (manual.isNotEmpty)
              ModalSection(
                title: 'Dashboard Steps',
                icon: Icons.checklist_rounded,
                child: Text(
                  manual,
                  style: const TextStyle(
                    color: AppColors.textPrimary,
                    fontSize: 13,
                    height: 1.5,
                  ),
                ),
              ),
          ],
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(c),
            child: const Text('Done'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Webhook Automation'),
      bottom: TabBar(
        controller: _tabs,
        indicatorColor: AppColors.teal,
        labelColor: AppColors.teal,
        unselectedLabelColor: AppColors.textMuted,
        tabs: const [
          Tab(icon: Icon(Icons.bolt_rounded), text: 'Connections'),
          Tab(icon: Icon(Icons.terminal_rounded), text: 'CLI Script'),
        ],
      ),
      actions: [
        if (widget.filterRadarId == null)
          Padding(
            padding: const EdgeInsets.only(right: 12),
            child: FilledButton.icon(
              onPressed: _busyAll || _loading ? null : _connectAll,
              icon: _busyAll
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: AppColors.bg,
                      ),
                    )
                  : const Icon(Icons.auto_fix_high_rounded, size: 18),
              label: const Text('Connect All'),
            ),
          ),
      ],
    ),
    body: _loading
        ? const Center(child: CircularProgressIndicator())
        : _error != null
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
        : TabBarView(
            controller: _tabs,
            children: [_buildPlansList(), _buildScriptTab()],
          ),
  );

  Widget _buildPlansList() {
    if (_plans.isEmpty) {
      return const Center(
        child: Text(
          'Create a radar first to connect webhooks.',
          style: TextStyle(color: AppColors.textMuted),
        ),
      );
    }
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 920),
        child: ListView.separated(
          padding: const EdgeInsets.all(16),
          itemCount: _plans.length,
          separatorBuilder: (_, _) => const SizedBox(height: 12),
          itemBuilder: (_, i) => _planCard(_plans[i]),
        ),
      ),
    );
  }

  Widget _planCard(Map<String, dynamic> p) {
    final supported = p['supported'] == true;
    final automatable = p['automatable'] == true;
    final connected = p['connected'] == true;
    final plugin = (p['plugin'] as String?) ?? '';
    final kind = (p['kind'] as String?) ?? 'provider';
    final err = (p['error'] as String?) ?? '';
    final note = (p['note'] as String?) ?? '';
    final busy = _busyKeys.contains(_planKey(p));

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Wrap(
                    spacing: 8,
                    runSpacing: 6,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      Text(
                        (p['radar_name'] as String?) ?? 'Radar',
                        style: const TextStyle(
                          fontSize: 15.5,
                          fontWeight: FontWeight.w700,
                          color: AppColors.textPrimary,
                        ),
                      ),
                      ProviderBadge(name: plugin, compact: true),
                      StatusPill(
                        label: kind == 'source' ? 'Git' : 'Deployments',
                        color: kind == 'source' ? AppColors.sky : AppColors.teal,
                      ),
                    ],
                  ),
                ),
                if (!supported)
                  const StatusPill(
                    label: 'Poll only',
                    color: AppColors.textMuted,
                    icon: Icons.schedule_rounded,
                  )
                else if (connected)
                  const StatusPill(
                    label: 'Connected',
                    color: AppColors.emerald,
                    icon: Icons.bolt_rounded,
                  )
                else
                  const StatusPill(
                    label: 'Not connected',
                    color: AppColors.amber,
                    icon: Icons.link_off_rounded,
                  ),
              ],
            ),
            if (note.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text(
                note,
                style: const TextStyle(color: AppColors.textMuted, fontSize: 12.5),
              ),
            ],
            if (err.isNotEmpty) ...[
              const SizedBox(height: 10),
              ModalBanner(
                message: err,
                color: AppColors.rose,
                icon: Icons.error_outline_rounded,
              ),
            ],
            if (supported) ...[
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (automatable)
                    FilledButton.icon(
                      onPressed: busy ? null : () => _connectOne(p),
                      icon: busy
                          ? const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: AppColors.bg,
                              ),
                            )
                          : Icon(
                              connected
                                  ? Icons.refresh_rounded
                                  : Icons.bolt_rounded,
                              size: 16,
                            ),
                      label: Text(connected ? 'Reconnect' : 'Auto-Connect'),
                    ),
                  if (automatable && plugin != 'appstore')
                    OutlinedButton.icon(
                      onPressed: busy
                          ? null
                          : () => _connectOne(p, askOverride: true),
                      icon: const Icon(Icons.key_rounded, size: 16),
                      label: const Text('1-Time Token'),
                    ),
                  OutlinedButton.icon(
                    onPressed: () => _showFallbackModal(p),
                    icon: const Icon(Icons.terminal_rounded, size: 16),
                    label: const Text('Manual / CLI'),
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildScriptTab() => Center(
    child: ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 920),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const ModalBanner(
              message:
                  'Fallback script: run any section locally if your stored credentials are strictly read-only or an API call is blocked.',
              color: AppColors.sky,
              icon: Icons.terminal_rounded,
            ),
            const SizedBox(height: 14),
            Expanded(
              child: CopyableCodeBlock(
                title: 'setup-webhooks.sh',
                text: _script,
                maxHeight: double.infinity,
              ),
            ),
          ],
        ),
      ),
    ),
  );
}
