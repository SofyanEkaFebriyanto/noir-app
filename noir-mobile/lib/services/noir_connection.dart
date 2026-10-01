import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

/// Status avatar — satu-satunya "UI state" yang ada. Tidak ada teks.
enum AvatarState { idle, listening, thinking, speaking }

AvatarState avatarStateFrom(String s) => AvatarState.values.firstWhere(
      (e) => e.name == s,
      orElse: () => AvatarState.idle,
    );

/// Klien WebSocket ke noir-brain. Auto-reconnect dengan backoff.
class NoirConnection {
  NoirConnection(this.url);

  final String url;
  WebSocketChannel? _ch;
  StreamSubscription? _sub;
  bool _disposed = false;
  int _retry = 0;
  Timer? _retryTimer;

  final _stateCtrl = StreamController<AvatarState>.broadcast();
  final _responseCtrl = StreamController<String>.broadcast();
  final _onlineCtrl = StreamController<bool>.broadcast();

  /// Perubahan status avatar dari server.
  Stream<AvatarState> get states => _stateCtrl.stream;

  /// Teks jawaban lengkap (untuk dibacakan via TTS, bukan ditampilkan).
  Stream<String> get responses => _responseCtrl.stream;

  /// true = terhubung ke brain.
  Stream<bool> get online => _onlineCtrl.stream;

  Future<void> connect() async {
    if (_disposed) return;
    try {
      _ch?.sink.close();
      await _sub?.cancel();
      _ch = WebSocketChannel.connect(Uri.parse(url));
      _sub = _ch!.stream.listen(
        _onMessage,
        onDone: _scheduleReconnect,
        onError: (_) => _scheduleReconnect(),
        cancelOnError: true,
      );
      _retry = 0;
      _onlineCtrl.add(true);
      send({'type': 'hello', 'client': 'noir-mobile/1.0'});
    } catch (_) {
      _scheduleReconnect();
    }
  }

  void _onMessage(dynamic raw) {
    try {
      final msg = jsonDecode(raw as String) as Map<String, dynamic>;
      switch (msg['type']) {
        case 'avatar_state':
          final state = avatarStateFrom(msg['state'] as String? ?? 'idle');
          _stateCtrl.add(state);
          final text = msg['text'] as String?;
          if (state == AvatarState.speaking &&
              text != null &&
              text.isNotEmpty) {
            _responseCtrl.add(text);
          }
        case 'token':
          break; // token streaming diabaikan di UI voice-only (nanti: dipakai untuk TTS dini)
        case 'error':
          _stateCtrl.add(AvatarState.idle);
      }
    } catch (_) {
      // abaikan frame rusak
    }
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _onlineCtrl.add(false);
    _retry++;
    final delay = Duration(seconds: _retry < 5 ? _retry * 2 : 10);
    _retryTimer?.cancel();
    _retryTimer = Timer(delay, connect);
  }

  /// Kirim transkrip STT ke brain.
  void sendTranscript(String text) {
    send({'type': 'chat', 'text': text});
  }

  /// Beri tahu server bahwa TTS selesai → server kembalikan status idle.
  void sendTtsDone() {
    send({'type': 'tts_done'});
  }

  /// Minta server membatalkan stream yang sedang jalan (interupsi / barge-in).
  void sendInterrupt() {
    send({'type': 'interrupt'});
  }

  void send(Map<String, dynamic> obj) {
    try {
      _ch?.sink.add(jsonEncode(obj));
    } catch (_) {}
  }

  Future<void> dispose() async {
    _disposed = true;
    _retryTimer?.cancel();
    await _sub?.cancel();
    await _ch?.sink.close();
    await _stateCtrl.close();
    await _responseCtrl.close();
    await _onlineCtrl.close();
  }
}
