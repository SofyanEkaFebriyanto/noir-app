import 'dart:async';

import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'services/noir_connection.dart';
import 'services/speech_pipeline.dart';
import 'services/tts_service.dart';
import 'services/wake_word_service.dart';
import 'settings_sheet.dart';
import 'widgets/avatar_renderer.dart';
import 'widgets/waveform_ring.dart';

/// Satu-satunya layar aplikasi: avatar + status. Nol teks.
///
/// State machine: idle → listening → thinking → speaking → idle.
/// Dipicu oleh wake word "Hey Noir" atau tap avatar (fallback).
/// Long-press avatar → pengaturan.
class VoiceScreen extends StatefulWidget {
  const VoiceScreen({super.key});

  @override
  State<VoiceScreen> createState() => _VoiceScreenState();
}

class _VoiceScreenState extends State<VoiceScreen> {
  AvatarState _state = AvatarState.idle;
  bool _online = false;

  NoirConnection? _conn;
  final _stt = SpeechPipeline();
  final _tts = TtsService();
  final _wake = WakeWordService();
  StreamSubscription? _subState;
  StreamSubscription? _subResp;
  StreamSubscription? _subOnline;

  @override
  void initState() {
    super.initState();
    _boot();
  }

  Future<void> _boot() async {
    final prefs = await SharedPreferences.getInstance();
    await _tts.init();
    await _tts.setRate(prefs.getDouble('tts_rate') ?? 0.95);
    _tts.onDone = () {
      _conn?.sendTtsDone();
      _toIdle();
    };
    await _stt.init();
    await _connectBrain();
  }

  /// (Re)koneksi ke brain. Dipakai saat boot dan setelah pengaturan berubah.
  Future<void> _connectBrain() async {
    final prefs = await SharedPreferences.getInstance();
    final wsUrl = prefs.getString('ws_url') ?? 'ws://100.0.0.1:8080/ws';

    await _subState?.cancel();
    await _subResp?.cancel();
    await _subOnline?.cancel();
    await _conn?.dispose();

    final conn = NoirConnection(wsUrl);
    _conn = conn;
    _subState = conn.states.listen((s) {
      if (mounted) setState(() => _state = s);
    });
    _subResp = conn.responses.listen(_onSpeakText);
    _subOnline = conn.online.listen((o) {
      if (mounted) setState(() => _online = o);
    });
    await conn.connect();
    await _startWakeWord();
  }

  Future<void> _startWakeWord() async {
    final prefs = await SharedPreferences.getInstance();
    final key = prefs.getString('porcupine_key') ?? '';
    if (key.isEmpty || _wake.running) return;
    try {
      await _wake.start(
        accessKey: key,
        keywordAssetPath: 'assets/hey-noir.ppn',
        onWake: _beginListening,
      );
    } catch (_) {
      // wake word opsional — tap avatar selalu tersedia
    }
  }

  void _toIdle() {
    if (mounted) setState(() => _state = AvatarState.idle);
    _startWakeWord(); // sesi selesai → nyalakan lagi wake word
  }

  /// Mulai mendengarkan: dari wake word atau tap avatar.
  Future<void> _beginListening() async {
    if (_state == AvatarState.listening || _state == AvatarState.thinking) return;
    if (!_online || _conn == null) return;
    await _wake.stop(); // hemat mic saat sesi aktif (v1: half-duplex)
    if (mounted) setState(() => _state = AvatarState.listening);
    await _tts.stop();
    await _stt.listen(onDone: (transcript) {
      if (mounted) setState(() => _state = AvatarState.thinking);
      _conn?.sendTranscript(transcript);
    });
  }

  /// Server mengirim teks jawaban lengkap → bacakan per kalimat.
  Future<void> _onSpeakText(String text) async {
    // _state sudah 'speaking' dari event avatar_state; tinggal bunyikan.
    await _tts.speakText(text);
  }

  Future<void> _openSettings() async {
    final changed = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      backgroundColor: const Color(0xFF141414),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => const SettingsSheet(),
    );
    if (changed == true) {
      final prefs = await SharedPreferences.getInstance();
      await _tts.setRate(prefs.getDouble('tts_rate') ?? 0.95);
      await _connectBrain();
    }
  }

  @override
  void dispose() {
    _subState?.cancel();
    _subResp?.cancel();
    _subOnline?.cancel();
    _wake.stop();
    _stt.stop();
    _tts.stop();
    _conn?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final vocal =
        _state == AvatarState.listening || _state == AvatarState.speaking;
    return Scaffold(
      backgroundColor: Colors.black,
      body: SafeArea(
        child: Stack(
          children: [
            // Avatar full-screen. Tap = bicara, long-press = pengaturan.
            GestureDetector(
              onTap: _beginListening,
              onLongPress: _openSettings,
              child: Center(
                child: WaveformRing(
                  active: vocal,
                  child: AspectRatio(
                    aspectRatio: 9 / 16,
                    child: AvatarRenderer(state: _state),
                  ),
                ),
              ),
            ),
            // Titik status koneksi (tanpa teks). Tap = retry.
            Positioned(
              top: 16,
              right: 16,
              child: GestureDetector(
                onTap: _connectBrain,
                child: Container(
                  width: 14,
                  height: 14,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: _online ? Colors.greenAccent : Colors.redAccent,
                    boxShadow: [
                      BoxShadow(
                        color: (_online ? Colors.greenAccent : Colors.redAccent)
                            .withOpacity(0.6),
                        blurRadius: 8,
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
