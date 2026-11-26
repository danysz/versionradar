import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api.dart';
import '../format.dart';
import '../theme.dart';
import '../widgets/google_sign_in_button.dart';
import '../widgets/modal.dart';

class LoginPage extends StatefulWidget {
  const LoginPage({super.key});
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  bool _busy = false;
  String? _error;

  Future<void> _google() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await pb
          .collection('users')
          .authWithOAuth2('google', (url) => launchUrl(url));
    } catch (e) {
      if (mounted) setState(() => _error = DisplayFormat.friendlyError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    body: Container(
      decoration: const BoxDecoration(
        gradient: RadialGradient(
          center: Alignment(0, -0.55),
          radius: 1.15,
          colors: [Color(0xFF102A43), AppColors.bg],
        ),
      ),
      child: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Container(
              padding: const EdgeInsets.all(32),
              decoration: BoxDecoration(
                color: AppColors.card.withValues(alpha: 0.92),
                borderRadius: BorderRadius.circular(24),
                border: Border.all(color: AppColors.border),
                boxShadow: [
                  BoxShadow(
                    color: AppColors.teal.withValues(alpha: 0.08),
                    blurRadius: 48,
                    spreadRadius: 4,
                  ),
                ],
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(
                    padding: const EdgeInsets.all(14),
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: AppColors.bg,
                      border: Border.all(
                        color: AppColors.teal.withValues(alpha: 0.45),
                      ),
                    ),
                    child: Image.asset('assets/logo.png', height: 72),
                  ),
                  const SizedBox(height: 18),
                  const Text(
                    'VersionRadar',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w800,
                      color: AppColors.textPrimary,
                      letterSpacing: -0.5,
                    ),
                  ),
                  const SizedBox(height: 6),
                  const Text(
                    'Every release & Git version, live on one radar.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: AppColors.textMuted, fontSize: 14),
                  ),
                  const SizedBox(height: 28),
                  if (_busy)
                    const Padding(
                      padding: EdgeInsets.symmetric(vertical: 10),
                      child: CircularProgressIndicator(),
                    )
                  else
                    GoogleSignInButton(onPressed: _google),
                  const SizedBox(height: 12),
                  const OutlinedButton(
                    onPressed: null,
                    child: Text('Sign in with Apple · coming soon'),
                  ),
                  if (_error != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 16),
                      child: ModalBanner(
                        message: _error!,
                        color: AppColors.rose,
                        icon: Icons.error_outline_rounded,
                      ),
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    ),
  );
}
