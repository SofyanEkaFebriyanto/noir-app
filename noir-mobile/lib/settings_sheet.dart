import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Pengaturan minimal — dibuka via long-press avatar.
/// Visual saja (ikon + field + slider), tanpa teks panjang, sesuai PRD F-07.
class SettingsSheet extends StatefulWidget {
  const SettingsSheet({super.key});

  @override
  State<SettingsSheet> createState() => _SettingsSheetState();
}

class _SettingsSheetState extends State<SettingsSheet> {
  final _wsCtrl = TextEditingController();
  final _keyCtrl = TextEditingController();
  double _rate = 0.95;
  bool _bargeInVoice = false;
  bool _loaded = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final p = await SharedPreferences.getInstance();
    _wsCtrl.text = p.getString('ws_url') ?? '';
    _keyCtrl.text = p.getString('porcupine_key') ?? '';
    _rate = p.getDouble('tts_rate') ?? 0.95;
    _bargeInVoice = p.getBool('barge_in_voice') ?? false;
    if (mounted) setState(() => _loaded = true);
  }

  Future<void> _save() async {
    final p = await SharedPreferences.getInstance();
    await p.setString('ws_url', _wsCtrl.text.trim());
    await p.setString('porcupine_key', _keyCtrl.text.trim());
    await p.setDouble('tts_rate', _rate);
    await p.setBool('barge_in_voice', _bargeInVoice);
    if (mounted) Navigator.pop(context, true); // true = config berubah
  }

  @override
  void dispose() {
    _wsCtrl.dispose();
    _keyCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.only(
          left: 24,
          right: 24,
          top: 12,
          bottom: MediaQuery.of(context).viewInsets.bottom + 24,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 40,
              height: 4,
              decoration: BoxDecoration(
                color: Colors.white24,
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const SizedBox(height: 16),
            if (!_loaded)
              const Padding(
                padding: EdgeInsets.all(24),
                child: CircularProgressIndicator(),
              )
            else ...[
              TextField(
                controller: _wsCtrl,
                decoration: const InputDecoration(
                  prefixIcon: Icon(Icons.dns_outlined),
                  hintText: 'ws://100.x.y.z:8080/ws',
                  border: OutlineInputBorder(),
                ),
                keyboardType: TextInputType.url,
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _keyCtrl,
                decoration: const InputDecoration(
                  prefixIcon: Icon(Icons.hearing_outlined),
                  hintText: 'Picovoice access key',
                  border: OutlineInputBorder(),
                ),
                obscureText: true,
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  const Icon(Icons.speed_outlined, color: Colors.white70),
                  Expanded(
                    child: Slider(
                      value: _rate,
                      min: 0.5,
                      max: 1.5,
                      divisions: 20,
                      onChanged: (v) => setState(() => _rate = v),
                    ),
                  ),
                ],
              ),
              Row(
                children: [
                  const Icon(Icons.record_voice_over_outlined,
                      color: Colors.white70),
                  const Expanded(
                    child: Padding(
                      padding: EdgeInsets.symmetric(horizontal: 12),
                      child: Text('Barge-in suara (eksperimental)',
                          style: TextStyle(color: Colors.white70)),
                    ),
                  ),
                  Switch(
                    value: _bargeInVoice,
                    onChanged: (v) => setState(() => _bargeInVoice = v),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              SizedBox(
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: _save,
                  icon: const Icon(Icons.check),
                  label: const Text('Simpan'),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
