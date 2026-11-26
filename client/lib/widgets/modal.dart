import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme.dart';

/// Polished, consistently formatted modal dialog with branded icon header,
/// scrollable structured content, and sticky footer action bar.
class AppModal extends StatelessWidget {
  const AppModal({
    super.key,
    required this.title,
    this.subtitle,
    this.icon,
    this.accentColor = AppColors.teal,
    required this.body,
    this.actions = const [],
    this.maxWidth = 520,
  });

  final String title;
  final String? subtitle;
  final IconData? icon;
  final Color accentColor;
  final Widget body;
  final List<Widget> actions;
  final double maxWidth;

  @override
  Widget build(BuildContext context) {
    final screenH = MediaQuery.sizeOf(context).height;
    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxWidth: maxWidth,
          maxHeight: screenH * 0.88,
        ),
        child: Container(
          decoration: BoxDecoration(
            color: AppColors.navy,
            borderRadius: BorderRadius.circular(22),
            border: Border.all(color: AppColors.border),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: 0.48),
                blurRadius: 36,
                offset: const Offset(0, 18),
              ),
            ],
          ),
          clipBehavior: Clip.antiAlias,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // Header
              Container(
                padding: const EdgeInsets.fromLTRB(20, 18, 12, 16),
                decoration: const BoxDecoration(
                  color: AppColors.card,
                  border: Border(bottom: BorderSide(color: AppColors.border)),
                ),
                child: Row(
                  children: [
                    if (icon != null) ...[
                      Container(
                        width: 40,
                        height: 40,
                        decoration: BoxDecoration(
                          color: accentColor.withValues(alpha: 0.14),
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(
                            color: accentColor.withValues(alpha: 0.35),
                          ),
                        ),
                        child: Icon(icon, color: accentColor, size: 20),
                      ),
                      const SizedBox(width: 14),
                    ],
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            title,
                            style: const TextStyle(
                              fontSize: 17,
                              fontWeight: FontWeight.w700,
                              color: AppColors.textPrimary,
                              letterSpacing: -0.2,
                            ),
                          ),
                          if (subtitle != null && subtitle!.isNotEmpty) ...[
                            const SizedBox(height: 2),
                            Text(
                              subtitle!,
                              style: const TextStyle(
                                fontSize: 12.5,
                                color: AppColors.textMuted,
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                    IconButton(
                      tooltip: 'Close',
                      onPressed: () => Navigator.of(context).pop(),
                      icon: const Icon(
                        Icons.close_rounded,
                        color: AppColors.textMuted,
                        size: 20,
                      ),
                    ),
                  ],
                ),
              ),
              // Scrollable body
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.all(20),
                  child: body,
                ),
              ),
              // Footer action bar
              if (actions.isNotEmpty)
                Container(
                  padding: const EdgeInsets.fromLTRB(20, 14, 20, 16),
                  decoration: const BoxDecoration(
                    color: AppColors.card,
                    border: Border(top: BorderSide(color: AppColors.border)),
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      for (var i = 0; i < actions.length; i++) ...[
                        if (i > 0) const SizedBox(width: 10),
                        actions[i],
                      ],
                    ],
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Visual card section inside a modal dialog to separate logical groups cleanly.
class ModalSection extends StatelessWidget {
  const ModalSection({
    super.key,
    required this.title,
    this.icon,
    this.trailing,
    required this.child,
  });

  final String title;
  final IconData? icon;
  final Widget? trailing;
  final Widget child;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: AppColors.card.withValues(alpha: 0.72),
      borderRadius: BorderRadius.circular(14),
      border: Border.all(color: AppColors.border),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            if (icon != null) ...[
              Icon(icon, size: 15, color: AppColors.teal),
              const SizedBox(width: 7),
            ],
            Expanded(
              child: Text(
                title.toUpperCase(),
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.8,
                  color: AppColors.teal,
                ),
              ),
            ),
            if (trailing != null) trailing!,
          ],
        ),
        const SizedBox(height: 10),
        child,
      ],
    ),
  );
}

/// Compact callout banner for info, warning, or error notes inside modals and pages.
class ModalBanner extends StatelessWidget {
  const ModalBanner({
    super.key,
    required this.message,
    required this.color,
    required this.icon,
  });

  final String message;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.11),
      borderRadius: BorderRadius.circular(12),
      border: Border.all(color: color.withValues(alpha: 0.35)),
    ),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, size: 16, color: color),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            message,
            style: TextStyle(color: color, fontSize: 12.5, height: 1.35),
          ),
        ),
      ],
    ),
  );
}

/// Clean single-line copyable field (e.g. Webhook URL or Secret) that never wraps into a giant wall of text.
class CompactCopyRow extends StatelessWidget {
  const CompactCopyRow({
    super.key,
    required this.label,
    required this.value,
    this.obscure = false,
  });

  final String label;
  final String value;
  final bool obscure;

  @override
  Widget build(BuildContext context) {
    final display = obscure && value.length > 10
        ? '${value.substring(0, 6)}••••••••${value.substring(value.length - 4)}'
        : value;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: AppColors.bg,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: AppColors.border),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  label,
                  style: const TextStyle(
                    fontSize: 11,
                    color: AppColors.textMuted,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  display,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 12.5,
                    color: AppColors.textPrimary,
                  ),
                ),
              ],
            ),
          ),
          IconButton(
            tooltip: 'Copy $label',
            visualDensity: VisualDensity.compact,
            icon: const Icon(
              Icons.copy_rounded,
              size: 16,
              color: AppColors.teal,
            ),
            onPressed: () {
              Clipboard.setData(ClipboardData(text: value));
              showAppToast(context, '$label copied');
            },
          ),
        ],
      ),
    );
  }
}

/// Styled floating toast notification with icon and optional details popup action.
void showAppToast(
  BuildContext context,
  String message, {
  bool isError = false,
  IconData? icon,
  String? actionLabel,
  VoidCallback? onAction,
}) {
  final color = isError ? AppColors.rose : AppColors.teal;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        content: Row(
          children: [
            Icon(
              icon ??
                  (isError
                      ? Icons.error_outline_rounded
                      : Icons.check_circle_outline_rounded),
              size: 18,
              color: color,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                message,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 13),
              ),
            ),
          ],
        ),
        action: actionLabel != null && onAction != null
            ? SnackBarAction(
                label: actionLabel,
                textColor: AppColors.teal,
                onPressed: onAction,
              )
            : null,
      ),
    );
}
