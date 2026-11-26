import 'package:flutter/material.dart';

/// Follows Google's branding guidelines for the dark-theme "Sign in with Google" button:
/// unmodified 4-colour G on a #131314 fill, 1px #8E918F outline, #E3E3E3 Roboto Medium 14 text,
/// 40px high, pill shape, 12px side padding and a 10px gap between logo (18px) and text.
class GoogleSignInButton extends StatelessWidget {
  const GoogleSignInButton({
    super.key,
    required this.onPressed,
    this.label = 'Sign in with Google',
  });
  final VoidCallback? onPressed;
  final String label;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null;
    return Opacity(
      opacity: enabled ? 1 : 0.38,
      child: Material(
        color: const Color(0xFF131314),
        shape: const StadiumBorder(side: BorderSide(color: Color(0xFF8E918F))),
        child: InkWell(
          customBorder: const StadiumBorder(),
          onTap: onPressed,
          child: SizedBox(
            height: 40,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Image.asset('assets/google_g.png', width: 18, height: 18),
                  const SizedBox(width: 10),
                  Text(
                    label,
                    style: const TextStyle(
                      fontFamily: 'Roboto',
                      fontSize: 14,
                      height: 20 / 14,
                      fontWeight: FontWeight.w500,
                      color: Color(0xFFE3E3E3),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
