import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme.dart';

/// Animated pulsing dot for live/in-progress deployments.
class PulsingDot extends StatefulWidget {
  const PulsingDot({super.key, required this.color, this.size = 10});
  final Color color;
  final double size;

  @override
  State<PulsingDot> createState() => _PulsingDotState();
}

class _PulsingDotState extends State<PulsingDot>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1100),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _ctrl,
    builder: (_, _) => Container(
      width: widget.size,
      height: widget.size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: widget.color,
        boxShadow: [
          BoxShadow(
            color: widget.color.withValues(alpha: 0.25 + 0.55 * _ctrl.value),
            blurRadius: 4 + 6 * _ctrl.value,
            spreadRadius: 1 + 2 * _ctrl.value,
          ),
        ],
      ),
    ),
  );
}

class StatusPill extends StatelessWidget {
  const StatusPill({
    super.key,
    required this.label,
    required this.color,
    this.icon,
    this.pulsing = false,
  });

  final String label;
  final Color color;
  final IconData? icon;
  final bool pulsing;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.14),
      borderRadius: BorderRadius.circular(999),
      border: Border.all(color: color.withValues(alpha: 0.4)),
    ),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (pulsing)
          PulsingDot(color: color, size: 8)
        else if (icon != null)
          Icon(icon, size: 14, color: color),
        if (pulsing || icon != null) const SizedBox(width: 6),
        Text(
          label,
          style: TextStyle(
            color: color,
            fontSize: 12,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    ),
  );
}

class ProviderBadge extends StatelessWidget {
  const ProviderBadge({super.key, required this.name, this.compact = false});
  final String name;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final m = providerMeta(name);
    return Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? 8 : 10,
        vertical: compact ? 3 : 5,
      ),
      decoration: BoxDecoration(
        color: m.color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: m.color.withValues(alpha: 0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(m.icon, size: compact ? 13 : 15, color: m.color),
          const SizedBox(width: 5),
          Text(
            m.label,
            style: TextStyle(
              color: m.color,
              fontSize: compact ? 11 : 12,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
  }
}

class BuildProgressBar extends StatelessWidget {
  const BuildProgressBar({
    super.key,
    required this.progress,
    required this.stage,
    required this.color,
  });

  final int progress;
  final String stage;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final pct = progress.clamp(0, 100);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(
              stage.isEmpty ? 'Running build...' : stage,
              style: TextStyle(
                color: color,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
            if (pct > 0)
              Text(
                '$pct%',
                style: TextStyle(
                  color: color,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
          ],
        ),
        const SizedBox(height: 6),
        ClipRRect(
          borderRadius: BorderRadius.circular(999),
          child: LinearProgressIndicator(
            value: pct > 0 ? pct / 100 : null,
            minHeight: 6,
            backgroundColor: AppColors.border,
            valueColor: AlwaysStoppedAnimation(color),
          ),
        ),
      ],
    );
  }
}

class CopyableCodeBlock extends StatelessWidget {
  const CopyableCodeBlock({
    super.key,
    required this.text,
    this.title,
    this.maxHeight = 160,
  });

  final String text;
  final String? title;
  final double maxHeight;

  @override
  Widget build(BuildContext context) => Container(
    decoration: BoxDecoration(
      color: AppColors.bg,
      borderRadius: BorderRadius.circular(12),
      border: Border.all(color: AppColors.border),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (title != null)
          Container(
            padding: const EdgeInsets.fromLTRB(12, 6, 6, 6),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: AppColors.border)),
            ),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    title!,
                    style: const TextStyle(
                      color: AppColors.textMuted,
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
                IconButton(
                  tooltip: 'Copy',
                  visualDensity: VisualDensity.compact,
                  icon: const Icon(Icons.copy_rounded, size: 16),
                  onPressed: () {
                    Clipboard.setData(ClipboardData(text: text));
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                        content: Text('Copied to clipboard'),
                        duration: Duration(seconds: 2),
                      ),
                    );
                  },
                ),
              ],
            ),
          ),
        ConstrainedBox(
          constraints: BoxConstraints(maxHeight: maxHeight),
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(12),
            child: SelectableText(
              text,
              style: const TextStyle(
                fontFamily: 'monospace',
                fontSize: 12,
                color: AppColors.textPrimary,
                height: 1.45,
              ),
            ),
          ),
        ),
      ],
    ),
  );
}
