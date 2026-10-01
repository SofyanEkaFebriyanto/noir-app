import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

import '../services/noir_connection.dart';

/// Menampilkan animasi avatar sesuai [AvatarState].
/// Semua aset di-preload sekali agar transisi antar status mulus.
class AvatarRenderer extends StatefulWidget {
  const AvatarRenderer({super.key, required this.state});

  final AvatarState state;

  /// Aset per status. Kalau file belum ada, fallback ke idle.
  static const assets = {
    AvatarState.idle: 'assets/avatar/idle.mp4',
    AvatarState.listening: 'assets/avatar/listening.mp4',
    AvatarState.thinking: 'assets/avatar/thinking.mp4',
    AvatarState.speaking: 'assets/avatar/speaking.mp4',
  };

  @override
  State<AvatarRenderer> createState() => _AvatarRendererState();
}

class _AvatarRendererState extends State<AvatarRenderer> {
  final Map<AvatarState, VideoPlayerController> _ctrls = {};
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _preload();
  }

  Future<void> _preload() async {
    for (final entry in AvatarRenderer.assets.entries) {
      final c = VideoPlayerController.asset(entry.value);
      try {
        await c.initialize();
        await c.setLooping(true);
        _ctrls[entry.key] = c;
      } catch (_) {
        // aset belum ada → nanti fallback ke idle
      }
    }
    if (!mounted) return;
    setState(() => _ready = true);
    _applyState();
  }

  @override
  void didUpdateWidget(covariant AvatarRenderer old) {
    super.didUpdateWidget(old);
    if (old.state != widget.state) _applyState();
  }

  void _applyState() {
    for (final entry in _ctrls.entries) {
      if (entry.key == widget.state || (entry.key == AvatarState.idle && !_ctrls.containsKey(widget.state))) {
        entry.value.play();
      } else {
        entry.value.pause();
      }
    }
  }

  @override
  void dispose() {
    for (final c in _ctrls.values) {
      c.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!_ready || _ctrls.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    final shown = _ctrls.containsKey(widget.state) ? widget.state : AvatarState.idle;
    return Stack(
      fit: StackFit.expand,
      children: [
        for (final entry in _ctrls.entries)
          Visibility(
            visible: entry.key == shown,
            maintainState: true,
            child: FittedBox(
              fit: BoxFit.cover,
              child: SizedBox(
                width: entry.value.value.size.width,
                height: entry.value.value.size.height,
                child: VideoPlayer(entry.value),
              ),
            ),
          ),
      ],
    );
  }
}
