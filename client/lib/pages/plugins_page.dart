import 'package:flutter/material.dart';
import 'package:pocketbase/pocketbase.dart';

import '../api.dart';
import '../format.dart';
import '../theme.dart';
import '../widgets/field_form.dart';
import '../widgets/modal.dart';
import '../widgets/ui_kit.dart';

class PluginsPage extends StatefulWidget {
  const PluginsPage({super.key});
  @override
  State<PluginsPage> createState() => _PluginsPageState();
}

class _PluginsPageState extends State<PluginsPage> {
  late Future<
    ({
      List<(Map<String, dynamic>, RecordModel?)> providers,
      List<(Map<String, dynamic>, RecordModel?)> sources,
      List<(Map<String, dynamic>, RecordModel?)> notifiers,
    })
  >
  _data = _load();

  Future<
    ({
      List<(Map<String, dynamic>, RecordModel?)> providers,
      List<(Map<String, dynamic>, RecordModel?)> sources,
      List<(Map<String, dynamic>, RecordModel?)> notifiers,
    })
  >
  _load() async {
    final catalog = await fetchPluginsCatalog();
    final mine = await pb.collection('user_plugins').getFullList();

    List<(Map<String, dynamic>, RecordModel?)> pair(List<dynamic>? raw) => [
      for (final p in (raw ?? const []).cast<Map<String, dynamic>>())
        (
          p,
          mine
              .where((r) => r.getStringValue('plugin_name') == p['name'])
              .firstOrNull,
        ),
    ];

    return (
      providers: pair(catalog['plugins'] as List?),
      sources: pair(catalog['sources'] as List?),
      notifiers: pair(catalog['notifiers'] as List?),
    );
  }

  Future<void> _edit(Map<String, dynamic> plugin, RecordModel? rec) async {
    final form = FieldForm(plugin['config_fields'] as List?);
    var enabled = rec?.getBoolValue('is_enabled') ?? true;
    String? error;
    final webhook = rec == null
        ? null
        : '$apiUrl/api/v1/webhooks/${pb.authStore.record!.id}/${plugin['name']}?product=<radar_id>';
    final meta = providerMeta(plugin['name'] as String);

    final save = await showDialog<bool>(
      context: context,
      builder: (c) => StatefulBuilder(
        builder: (c, set) => AppModal(
          title: meta.label,
          subtitle: '${meta.tagline} · v${plugin['version']}',
          icon: meta.icon,
          accentColor: meta.color,
          maxWidth: 500,
          body: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              ModalSection(
                title: 'Status',
                icon: Icons.power_settings_new_rounded,
                trailing: Switch(
                  value: enabled,
                  onChanged: (v) => set(() => enabled = v),
                ),
                child: Text(
                  enabled
                      ? 'Active for scheduled polling and incoming webhooks.'
                      : 'Paused — excluded from polling and webhooks.',
                  style: const TextStyle(
                    color: AppColors.textMuted,
                    fontSize: 12.5,
                  ),
                ),
              ),
              const SizedBox(height: 14),
              ModalSection(
                title: 'Credentials',
                icon: Icons.lock_outline_rounded,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (rec != null) ...[
                      const ModalBanner(
                        message:
                            'Stored with AES-256-GCM encryption. Leave fields empty to keep your existing credentials.',
                        color: AppColors.teal,
                        icon: Icons.verified_user_outlined,
                      ),
                      const SizedBox(height: 4),
                    ],
                    ...form.inputs(),
                  ],
                ),
              ),
              if (error != null) ...[
                const SizedBox(height: 12),
                ModalBanner(
                  message: error!,
                  color: AppColors.rose,
                  icon: Icons.error_outline_rounded,
                ),
              ],
              if (webhook != null && plugin['webhook_supported'] == true) ...[
                const SizedBox(height: 14),
                ModalSection(
                  title: 'Webhook Endpoint',
                  icon: Icons.bolt_rounded,
                  child: Column(
                    children: [
                      CompactCopyRow(label: 'Endpoint URL', value: webhook),
                      const SizedBox(height: 8),
                      CompactCopyRow(
                        label: 'Signing Secret',
                        value: rec!.getStringValue('webhook_secret'),
                        obscure: true,
                      ),
                    ],
                  ),
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
              icon: const Icon(Icons.check_rounded, size: 18),
              onPressed: () {
                final (_, err) = form.values();
                if (err != null && !(rec != null && form.isEmpty)) {
                  set(() => error = err);
                } else {
                  Navigator.pop(c, true);
                }
              },
              label: const Text('Save'),
            ),
          ],
        ),
      ),
    );
    if (save != true) return;
    try {
      await api(
        '/plugins/${plugin['name']}/config',
        method: 'POST',
        body: {
          'is_enabled': enabled,
          if (!form.isEmpty) 'config': form.values().$1,
        },
      );
      setState(() => _data = _load());
      if (mounted) {
        showAppToast(context, '${meta.label} configuration saved');
      }
    } catch (e) {
      if (mounted) {
        showAppToast(context, DisplayFormat.friendlyError(e), isError: true);
      }
    }
  }

  Widget _sectionHeader(String title, String subtitle) => Padding(
    padding: const EdgeInsets.fromLTRB(4, 16, 4, 10),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: AppColors.textPrimary,
          ),
        ),
        const SizedBox(height: 2),
        Text(
          subtitle,
          style: const TextStyle(fontSize: 13, color: AppColors.textMuted),
        ),
      ],
    ),
  );

  Widget _pluginCard(Map<String, dynamic> p, RecordModel? rec) {
    final name = p['name'] as String;
    final meta = providerMeta(name);
    final isEnabled = rec?.getBoolValue('is_enabled') == true;
    final supportsWebhook = p['webhook_supported'] == true;
    final isNotifier = p['kind'] == 'notifier';
    final locked = p['locked'] == true;

    return Card(
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: locked
            ? () => showAppToast(
                context,
                '${meta.label} requires VersionRadar Pro',
                icon: Icons.workspace_premium_rounded,
              )
            : () => _edit(p, rec),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: meta.color.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: meta.color.withValues(alpha: 0.35)),
                ),
                child: Icon(meta.icon, color: meta.color, size: 22),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          meta.label,
                          style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: AppColors.textPrimary,
                          ),
                        ),
                        const SizedBox(width: 8),
                        Text(
                          'v${p['version']}',
                          style: const TextStyle(
                            fontSize: 11.5,
                            color: AppColors.textMuted,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 2),
                    Text(
                      meta.tagline,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 12.5,
                        color: AppColors.textMuted,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Wrap(
                      spacing: 6,
                      runSpacing: 4,
                      children: [
                        if (locked)
                          const StatusPill(
                            label: 'PRO',
                            color: AppColors.amber,
                            icon: Icons.workspace_premium_rounded,
                          )
                        else
                          StatusPill(
                            label: isEnabled
                                ? 'Enabled'
                                : rec == null
                                ? 'Not configured'
                                : 'Paused',
                            color: isEnabled
                                ? AppColors.emerald
                                : AppColors.textMuted,
                          ),
                        if (isNotifier)
                          const StatusPill(
                            label: 'Outbound Alert',
                            color: AppColors.amber,
                            icon: Icons.notifications_active_outlined,
                          )
                        else
                          StatusPill(
                            label: supportsWebhook
                                ? 'Webhooks + Poll'
                                : 'Poll only',
                            color: supportsWebhook
                                ? AppColors.teal
                                : AppColors.sky,
                            icon: supportsWebhook
                                ? Icons.bolt_rounded
                                : Icons.schedule_rounded,
                          ),
                      ],
                    ),
                  ],
                ),
              ),
              const Icon(
                Icons.chevron_right_rounded,
                color: AppColors.textMuted,
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Plugins & Sources')),
    body: FutureBuilder(
      future: _data,
      builder: (_, snap) {
        if (snap.hasError) {
          return Center(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: ModalBanner(
                message: DisplayFormat.friendlyError(snap.error!),
                color: AppColors.rose,
                icon: Icons.error_outline_rounded,
              ),
            ),
          );
        }
        if (!snap.hasData) {
          return const Center(child: CircularProgressIndicator());
        }
        final d = snap.data!;
        return Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 860),
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 32),
              children: [
                _sectionHeader(
                  'Deployment Providers',
                  'Where your apps and services are built and published.',
                ),
                for (final (p, rec) in d.providers) ...[
                  _pluginCard(p, rec),
                  const SizedBox(height: 10),
                ],
                _sectionHeader(
                  'Git Version Sources',
                  'Compare deployed versions against tags, branches, or version files.',
                ),
                for (final (s, rec) in d.sources) ...[
                  _pluginCard(s, rec),
                  const SizedBox(height: 10),
                ],
                if (d.notifiers.isNotEmpty) ...[
                  _sectionHeader(
                    'Alert Notifiers',
                    'Send instant notifications when deployments finish or fail.',
                  ),
                  for (final (n, rec) in d.notifiers) ...[
                    _pluginCard(n, rec),
                    const SizedBox(height: 10),
                  ],
                ],
              ],
            ),
          ),
        );
      },
    ),
  );
}
