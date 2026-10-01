import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'services/barge_in.dart';
import 'services/noir_connection.dart';
import 'services/speech_pipeline.dart';
import 'services/tts_service.dart';
import 'services/wake_word_service.dart';
import 'settings_sheet.dart';
import 'widgets/avatar_renderer.dart';
import 'widgets/waveform_ring.dart';

/// Satu-satunya layar aplikasi: avatar + status. Nol teks.
///
/// v1.1 full duplex:
/// - Tap avatar saat Noir bicara = interupsi (berhenti + langsung dengarkan).
/// - Barge-in suara (eksperimental, default mati di pengaturan).
/// - Continuous conversation: 30 dtk setelah Noir selesai, ngomong aja
///   tanpa "Hey Noir".
/// - Perintah lokal tanpa LLM: "diam"/"stop" berhenti total,
///   "ulangi" baca ulang jawaban terakhir.
/// - Diam 12 dtk saat listening → kembali idle.
class VoiceScreen extends StatefulWidget {
  const VoiceScreen({super.key});

  @override
  State<VoiceScreen> createState() => _VoiceScreenState();
}

class _VoiceScreenState extends State<VoiceScreen> {
  static const _conversationWindow = Duration(seconds: 30);
  static const _listenTimeout = Duration(seconds: 12);
  static final _stopCmd = RegExp(r'^(diam|stop|berhenti|udah|udahan|sudah)$');
  static final _repeatCmd = RegExp(r'^ulangi(\s+(dong|ya|lagi))?$');

  AvatarState _state = AvatarState.idle;
  bool _online = false;

  NoirConnection? _conn;
  final _stt = SpeechPipeline();
  final _tts = TtsService();
  final _wake = WakeWordService();
  late final BargeInMonitor _barge;
  StreamSubscription? _subState;
  StreamSubscription? _subResp;
  StreamSubscription? _subOnline;
  Timer? _listenTimer;
  DateTime _conversationUntil = DateTime.fromMillisecondsSinceEpoch(0);
  String? _lastResponse;

  @override
  void initState() {
    super.initState();
    _barge = BargeInMonitor(_stt);
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
    if (kDebugMode) {
      // Bantuan debug: tampilkan error TTS sebagai snackbar (rilis tetap nol teks).
      _tts.onError = (msg) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('TTS error: $msg')),
        );
      };
    }
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
    _listenTimer?.cancel();
    _barge.stop();
    // Continuous conversation: masih dalam window → langsung dengarkan lagi,
    // tanpa perlu "Hey Noir".
    if (DateTime.now().isBefore(_conversationUntil)) {
      _beginListening();
      return;
    }
    if (mounted) setState(() => _state = AvatarState.idle);
    _startWakeWord();
  }

  /// Mulai mendengarkan: dari wake word, tap avatar, interupsi, atau
  /// continuous conversation.
  Future<void> _beginListening() async {
    if (_state == AvatarState.listening || _state == AvatarState.thinking) {
      return;
    }
    if (!_online || _conn == null) return;
    await _wake.stop(); // hemat mic saat sesi aktif
    await _barge.stop();
    _conn!.sendInterrupt(); // batalkan stream server kalau masih jalan
    if (mounted) setState(() => _state = AvatarState.listening);
    await _tts.stop();
    _listenTimer?.cancel();
    _listenTimer = Timer(_listenTimeout, () {
      // Diam 12 dtk tanpa suara → kembali idle (tidak menggantung di listening).
      if (_state == AvatarState.listening && !_stt.hasSpeech) {
        _stt.stop();
        _toIdle();
      }
    });
    await _stt.listen(onDone: (transcript) {
      _listenTimer?.cancel();
      _onUserSpeech(transcript);
    });
  }

  /// Ucapan pengguna selesai ditranskrip: perintah lokal dulu, baru ke server.
  void _onUserSpeech(String transcript) {
    final t = transcript.toLowerCase().trim();

    // Perintah lokal — tanpa LLM, tanpa network.
    if (_stopCmd.hasMatch(t)) {
      _conversationUntil =
          DateTime.fromMillisecondsSinceEpoch(0); // akhiri sesi paksa
      _conn?.sendInterrupt();
      _tts.stop();
      _toIdle();
      return;
    }
    if (_repeatCmd.hasMatch(t) && _lastResponse != null) {
      _speak(_lastResponse!);
      return;
    }

    // Percakapan biasa → perpanjang jendela continuous conversation.
    _conversationUntil = DateTime.now().add(_conversationWindow);
    if (mounted) setState(() => _state = AvatarState.thinking);
    _conn?.sendTranscript(transcript);
  }

  /// Server mengirim teks jawaban lengkap → bacakan per kalimat.
  Future<void> _onSpeakText(String text) async {
    _lastResponse = text;
    await _speak(text);
  }

  Future<void> _speak(String text) async {
    if (mounted) setState(() => _state = AvatarState.speaking);
    final prefs = await SharedPreferences.getInstance();
    if (prefs.getBool('barge_in_voice') ?? false) {
      await _barge.start(fullText: text, onBargeIn: _onVoiceBargeIn);
    }
    await _tts.speakText(text);
  }

  /// Barge-in suara terdeteksi → perlakukan sebagai ucapan baru.
  void _onVoiceBargeIn() {
    _beginListening();
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
    _listenTimer?.cancel();
    _subState?.cancel();
    _subResp?.cancel();
    _subOnline?.cancel();
    _wake.stop();
    _barge.stop();
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
            // Avatar full-screen. Tap = bicara / interupsi, long-press = pengaturan.
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
                            .withValues(alpha: 0.6),
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
