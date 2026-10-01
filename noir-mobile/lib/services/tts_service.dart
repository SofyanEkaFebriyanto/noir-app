import 'package:flutter_tts/flutter_tts.dart';

/// TTS on-device dengan antrean per kalimat.
///
/// Trik latency voice-to-voice: kalimat pertama dibunyikan segera setelah
/// diterima, tanpa menunggu jawaban lengkap. Server mengirim teks utuh,
/// [speakText] memecahnya per kalimat dan mengantrekannya.
class TtsService {
  final FlutterTts _tts = FlutterTts();
  final List<String> _queue = [];
  bool _busy = false;

  /// Dipanggil saat kalimat pertama mulai bunyi.
  void Function()? onStart;

  /// Dipanggil saat seluruh antrean habis dibacakan.
  void Function()? onDone;

  Future<void> init() async {
    await _tts.setLanguage('id-ID');
    await _tts.setSpeechRate(0.95);
    await _tts.setPitch(1.0);
    _tts.setStartHandler(() => onStart?.call());
    _tts.setCompletionHandler(_next);
    _tts.setCancelHandler(_next);
    _tts.setErrorHandler((_) => _next());
  }

  /// Pecah teks jadi kalimat. Aturan sederhana yang aman untuk TTS id-ID.
  static List<String> splitSentences(String text) {
    return text
        .split(RegExp(r'(?<=[.!?…])\s+'))
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
  }

  Future<void> speakText(String text) => speakSentences(splitSentences(text));

  Future<void> speakSentences(List<String> sentences) async {
    _queue.addAll(sentences.where((s) => s.trim().isNotEmpty));
    if (!_busy) _next();
  }

  Future<void> _next() async {
    if (_queue.isEmpty) {
      _busy = false;
      onDone?.call();
      return;
    }
    _busy = true;
    await _tts.speak(_queue.removeAt(0));
  }

  /// Hentikan bicara + kosongkan antrean (dipakai saat user interupsi / v1.1 barge-in).
  Future<void> stop() async {
    _queue.clear();
    _busy = false;
    await _tts.stop();
  }

  Future<void> setRate(double rate) => _tts.setSpeechRate(rate);
}
