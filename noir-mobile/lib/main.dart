import 'package:flutter/material.dart';

import 'voice_screen.dart';

void main() => runApp(const NoirApp());

/// Entry point aplikasi Noir: satu layar, full voice, nol teks.
class NoirApp extends StatelessWidget {
  const NoirApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Noir',
      debugShowCheckedModeBanner: false,
      theme: ThemeData.dark(useMaterial3: true),
      home: const VoiceScreen(),
    );
  }
}
