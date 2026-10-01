import 'package:flutter/material.dart';

/// Cincin waveform berdenyut di sekeliling avatar saat listening/speaking.
/// Satu-satunya indikator visual selain avatar — tanpa teks.
class WaveformRing extends StatefulWidget {
  const WaveformRing({super.key, required this.active, required this.child});

  final bool active;
  final Widget child;

  @override
  State<WaveformRing> createState() => _WaveformRingState();
}

class _WaveformRingState extends State<WaveformRing>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    );
    if (widget.active) _ctrl.repeat(reverse: true);
  }

  @override
  void didUpdateWidget(covariant WaveformRing old) {
    super.didUpdateWidget(old);
    if (widget.active && !_ctrl.isAnimating) {
      _ctrl.repeat(reverse: true);
    } else if (!widget.active && _ctrl.isAnimating) {
      _ctrl.stop();
      _ctrl.reset();
    }
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _ctrl,
      builder: (context, child) {
        final t = _ctrl.value;
        return Container(
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            boxShadow: widget.active
                ? [
                    BoxShadow(
                      color: Colors.cyanAccent.withOpacity(0.35 + 0.3 * t),
                      blurRadius: 30 + 40 * t,
                      spreadRadius: 4 + 10 * t,
                    ),
                  ]
                : null,
          ),
          child: ClipOval(child: child),
        );
      },
      child: widget.child,
    );
  }
}
