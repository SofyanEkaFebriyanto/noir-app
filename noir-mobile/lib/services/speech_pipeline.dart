import 'dart:async';

import 'package:speech_to_text/speech_to_text.dart';

/// STT streaming on-device + endpointing otomatis.
///
/// Cara pakai: [listen] → user ngomong → diam 0,8 dtk → [onDone] dipanggil
/// dengan transkrip final. Audio tidak pernah keluar dari HP.
class SpeechPipeline {
  final SpeechToText _stt = SpeechToText();
  Timer? _endpointTimer;
  String _buffer = '';
  bool _done = false;

  Future<bool> init() => _stt.initialize();

  bool get available => _stt.isAvailable;
  bool get listening => _stt.isListening;

  Future<void> listen({
    required void Function(String transcript) onDone,
    Duration endpointing = const Duration(milliseconds: 800),
    String localeId = 'id_ID',
  }) async {
    _buffer = '';
    _done = false;
    await _stt.listen(
      onResult: (result) {
        _buffer = result.recognizedWords;
        _endpointTimer?.cancel();
        if (result.finalResult) {
          _finish(onDone);
          return;
        }
        // Endpointing: anggap selesai kalau user diam selama [endpointing].
        _endpointTimer = Timer(endpointing, () => _finish(onDone));
      },
      listenMode: ListenMode.dictation,
      partialResults: true,
      localeId: localeId,
      cancelOnError: true,
    );
  }

  void _finish(void Function(String) onDone) {
    if (_done) return;
    _done = true;
    _endpointTimer?.cancel();
    final text = _buffer.trim();
    stop();
    if (text.isNotEmpty) onDone(text);
  }

  Future<void> stop() async {
    _endpointTimer?.cancel();
    if (_stt.isListening) await _stt.stop();
  }
}
