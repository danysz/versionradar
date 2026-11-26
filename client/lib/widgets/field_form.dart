import 'dart:convert';

import 'package:flutter/material.dart';

/// One input described by the server (`config_fields` / `target_fields` / `version_strategies`).
class FieldSpec {
  FieldSpec(Map<String, dynamic> m)
    : key = m['key'] as String,
      label = m['label'] as String,
      help = m['help'] as String?,
      secret = m['secret'] == true,
      multiline = m['multiline'] == true,
      json = m['json'] == true,
      optional = m['optional'] == true,
      defaultValue = m['default'] as String?,
      options = ((m['options'] as List?) ?? const []).cast<String>();

  final String key, label;
  final String? help, defaultValue;
  final bool secret, multiline, json, optional;
  final List<String> options;
}

/// Turns server field descriptions into Material inputs and back into the JSON map to store.
class FieldForm {
  FieldForm(List<dynamic>? raw, {Map<String, dynamic>? initialValues})
    : specs = [
        for (final m in raw ?? const [])
          FieldSpec((m as Map).cast<String, dynamic>()),
      ] {
    for (final s in specs) {
      final init = initialValues?[s.key];
      String text = '';
      if (init != null) {
        text = init is String ? init : jsonEncode(init);
      } else if (s.defaultValue != null && s.defaultValue!.isNotEmpty) {
        text = s.defaultValue!;
      }
      controllers[s.key] = TextEditingController(text: text);
    }
  }

  final List<FieldSpec> specs;
  final Map<String, TextEditingController> controllers = {};

  bool get isEmpty => controllers.values.every((c) => c.text.trim().isEmpty);

  bool get allRequiredEmpty => specs
      .where((s) => !s.optional)
      .every((s) => controllers[s.key]!.text.trim().isEmpty);

  List<Widget> inputs({String? keepHint}) => [
    for (final s in specs)
      Padding(
        padding: const EdgeInsets.only(top: 12),
        child: s.options.isNotEmpty
            ? DropdownButtonFormField<String>(
                initialValue:
                    s.options.contains(controllers[s.key]!.text)
                        ? controllers[s.key]!.text
                        : s.options.first,
                decoration: InputDecoration(
                  labelText: s.optional ? '${s.label} (optional)' : s.label,
                  helperText: s.help ?? keepHint,
                  helperMaxLines: 2,
                ),
                items: [
                  for (final o in s.options)
                    DropdownMenuItem(value: o, child: Text(o)),
                ],
                onChanged: (v) {
                  if (v != null) controllers[s.key]!.text = v;
                },
              )
            : TextField(
                controller: controllers[s.key],
                obscureText: s.secret && !s.multiline,
                minLines: s.multiline ? 3 : 1,
                maxLines: s.multiline ? 6 : 1,
                decoration: InputDecoration(
                  labelText: s.label,
                  helperText: s.help ?? keepHint,
                  helperMaxLines: 2,
                ),
              ),
      ),
  ];

  /// Returns the map to store, or an error message if a required field is missing/invalid.
  (Map<String, dynamic>?, String?) values() {
    final out = <String, dynamic>{};
    for (final s in specs) {
      final t = controllers[s.key]!.text.trim();
      if (t.isEmpty) {
        if (s.optional) continue;
        return (null, '${s.label} is required');
      }
      if (s.json) {
        try {
          out[s.key] = jsonDecode(t);
        } catch (_) {
          return (
            null,
            '${s.label} must be valid JSON (paste the whole key file)',
          );
        }
      } else {
        out[s.key] = t;
      }
    }
    return (out, null);
  }
}
