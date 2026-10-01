import 'speech_pipeline.dart';

/// Monitor barge-in suara — EKSPERIMENTAL (default mati, aktifkan di pengaturan).
///
/// Masalahnya: tanpa echo cancellation, mic ikut mendengar suara Noir sendiri
/// dari speaker saat TTS bunyi. Heuristik di sini:
/// - transkrip parsial yang mayoritas katanya cocok dengan teks yang sedang
///   dibacakan → dianggap suara Noir sendiri → diabaikan;
/// - dua parsial beruntun yang TIDAK cocok → itu suara pengguna → barge-in.
///
/// Jujur: ini tidak 100% andal. Tap avatar saat Noir bicara tetap jalur
/// interupsi utama yang selalu bisa diandalkan.
class BargeInMonitor {
  BargeInMonitor(this._stt);

  final SpeechPipeline _stt;
  bool _active = false;
  String _expected = '';
  int _diffStreak = 0;
  void Function()? _onBargeIn;

  bool get active => _active;

  /// Mulai monitor selama [fullText] dibacakan. [onBargeIn] dipanggil sekali
  /// saat suara pengguna terdeteksi.
  Future<void> start({
    required String fullText,
    required void Function() onBargeIn,
  }) async {
    await stop();
    _expected = _normalize(fullText);
    _diffStreak = 0;
    _onBargeIn = onBargeIn;
    _active = true;
    await _stt.listen(
      onDone: (_) {}, // diabaikan dalam mode monitor
      onPartial: _check,
      autoFinish: false,
    );
  }

  void _check(String partial) {
    if (!_active) return;
    final p = _normalize(partial);
    if (p.isEmpty) return;
    if (_isOwnVoice(p)) {
      _diffStreak = 0;
      return;
    }
    if (++_diffStreak >= 2) {
      final cb = _onBargeIn;
      _active = false;
      _diffStreak = 0;
      _onBargeIn = null;
      cb?.call();
    }
  }

  /// true jika mayoritas kata parsial muncul di teks yang sedang dibacakan.
  bool _isOwnVoice(String partial) {
    final words = partial.split(' ').where((w) => w.length > 2).toList();
    if (words.isEmpty) return true; // terlalu pendek → abaikan
    var hits = 0;
    for (final w in words) {
      if (_expected.contains(w)) hits++;
    }
    return hits * 2 >= words.length;
  }

  static String _normalize(String s) {
    return s
        .toLowerCase()
        .replaceAll(RegExp(r'[^a-z0-9 ]'), ' ')
        .replaceAll(RegExp(r'\s+'), ' ')
        .trim();
  }

  Future<void> stop() async {
    _active = false;
    _onBargeIn = null;
    _diffStreak = 0;
    await _stt.stop();
  }
}
