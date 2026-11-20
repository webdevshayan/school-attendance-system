import 'package:flutter/material.dart';

import '../core/pairing/pairing_service.dart';

/// Bottom sheet that collects the school's pairing code and binds this
/// install to the child. On success [onPaired] fires with the student id.
class PairingSheet extends StatefulWidget {
  const PairingSheet({
    super.key,
    required this.service,
    required this.onPaired,
  });

  final PairingService service;
  final Future<void> Function(String studentId) onPaired;

  @override
  State<PairingSheet> createState() => _PairingSheetState();
}

class _PairingSheetState extends State<PairingSheet> {
  final _codeController = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _codeController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final code = _codeController.text.trim();
    if (code.isEmpty) {
      setState(() => _error = 'Enter the code from the school.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    final ok = await widget.service.pair(code);
    if (!mounted) return;
    if (!ok) {
      setState(() {
        _busy = false;
        _error =
            'That code did not work. Check it and try again (codes expire '
            'after 7 days).';
      });
      return;
    }
    final studentId = await widget.service.pairedStudentId();
    if (!mounted) return;
    Navigator.of(context).pop();
    if (studentId != null) {
      await widget.onPaired(studentId);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(
        left: 24,
        right: 24,
        top: 24,
        bottom: MediaQuery.of(context).viewInsets.bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Pair this device',
            style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          const Text(
            'Ask the school for your child\'s pairing code and enter it '
            'here. Both guardians can use the same code.',
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _codeController,
            textCapitalization: TextCapitalization.characters,
            autofocus: true,
            decoration: InputDecoration(
              labelText: 'Pairing code',
              hintText: 'e.g. ABCD2345',
              errorText: _error,
            ),
            onSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: FilledButton.icon(
              icon: _busy
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.link),
              label: Text(_busy ? 'Pairing...' : 'Pair this device'),
              onPressed: _busy ? null : _submit,
            ),
          ),
        ],
      ),
    );
  }
}
