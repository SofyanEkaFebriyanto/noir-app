import 'package:porcupine_flutter/porcupine_manager.dart';

/// Wake word "Hey Noir" via Porcupine (100% on-device).
///
/// Setup sekali:
/// 1. Daftar gratis di Picovoice Console (console.picovoice.ai)
/// 2. Latih custom keyword "Hey Noir" → download file `.ppn`
/// 3. Taruh di `assets/` dan daftarkan di pubspec, isi accessKey di bawah
///    (atau via SettingsStore nanti).
class WakeWordService {
  PorcupineManager? _manager;
  bool get running => _manager != null;

  /// Mulai mendengarkan wake word. [onWake] dipanggil saat terdeteksi.
  Future<void> start({
    required String accessKey,
    required String keywordAssetPath,
    required void Function() onWake,
  }) async {
    await stop();
    _manager = await PorcupineManager.fromKeywordPaths(
      accessKey,
      [keywordAssetPath],
      (_) => onWake(),
    );
    await _manager!.start();
  }

  Future<void> stop() async {
    await _manager?.stop();
    await _manager?.delete();
    _manager = null;
  }
}
