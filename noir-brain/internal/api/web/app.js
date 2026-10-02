/* Noir web client — voice loop over the same WebSocket protocol as noir-mobile. */
(() => {
  'use strict';
  const body = document.body;
  const avatar = document.getElementById('avatar');
  const statusEl = document.getElementById('status');
  const connEl = document.getElementById('conn');

  let ws = null;
  let state = 'idle'; // idle | listening | thinking | speaking
  let recog = null;
  let voices = [];
  let speakQueue = [];
  let speaking = false;

  function setState(s, label) {
    state = s;
    body.dataset.state = s;
    if (label !== undefined) statusEl.textContent = label;
  }

  /* ---------- TTS voices ---------- */
  function loadVoices() { voices = window.speechSynthesis ? speechSynthesis.getVoices() : []; }
  if ('speechSynthesis' in window) {
    loadVoices();
    speechSynthesis.onvoiceschanged = loadVoices;
  }
  function pickVoice() {
    const prefs = ['id-id', 'id_id', 'id'];
    for (const p of prefs) {
      const v = voices.find(v => (v.lang || '').toLowerCase().replace('_', '-') === p);
      if (v) return v;
    }
    for (const p of prefs) {
      const v = voices.find(v => (v.lang || '').toLowerCase().startsWith(p.split('-')[0]));
      if (v) return v;
    }
    return voices.find(v => (v.lang || '').toLowerCase().startsWith('en')) || null;
  }

  /* ---------- WebSocket ---------- */
  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(proto + '//' + location.host + '/ws');
    ws.onopen = () => {
      connEl.classList.add('on');
      send({ type: 'hello', client: 'noir-web/1.0' });
    };
    ws.onclose = () => {
      connEl.classList.remove('on');
      setState('idle', 'terputus — menyambung ulang…');
      setTimeout(connect, 2000);
    };
    ws.onerror = () => { try { ws.close(); } catch (e) {} };
    ws.onmessage = (e) => {
      let m; try { m = JSON.parse(e.data); } catch (err) { return; }
      onEvent(m);
    };
  }
  function send(o) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(o)); }

  function onEvent(m) {
    switch (m.type) {
      case 'avatar_state':
        if (m.state === 'thinking') setState('thinking', 'berpikir…');
        else if (m.state === 'speaking' && m.text) speakText(m.text);
        else if (m.state === 'idle' && !speaking) setState('idle', 'ketuk untuk bicara');
        break;
      case 'token': /* streaming — diabaikan di web MVP */ break;
      case 'done': break;
      case 'proactive_ping': showPing(m.text); break;
      case 'error':
        setState('idle', 'error: ' + (m.error || 'unknown'));
        break;
    }
  }

  /* ---------- TTS (browser speechSynthesis) ---------- */
  function splitSentences(t) {
    const parts = t.match(/[^.!?…\n]+[.!?…\n]*/g);
    if (!parts) return [t];
    return parts.map(s => s.trim()).filter(Boolean);
  }
  function speakText(text) {
    if (!('speechSynthesis' in window)) { send({ type: 'tts_done' }); setState('idle', 'ketuk untuk bicara'); return; }
    speechSynthesis.cancel();
    speakQueue = splitSentences(text);
    speaking = true;
    setState('speaking', 'Noir bicara…');
    speakNext();
  }
  function speakNext() {
    const s = speakQueue.shift();
    if (!s) { speaking = false; send({ type: 'tts_done' }); return; }
    const u = new SpeechSynthesisUtterance(s);
    u.lang = 'id-ID';
    const v = pickVoice(); if (v) u.voice = v;
    u.rate = 1.0; u.volume = 1.0;
    u.onend = speakNext;
    u.onerror = speakNext;
    speechSynthesis.speak(u);
  }

  /* ---------- F-14: sapaan proaktif (in-app only) ---------- */
  const pingToast = document.getElementById('ping-toast');
  const pingText = pingToast ? pingToast.querySelector('.ping-text') : null;
  let pingTimer = null;
  function showPing(text) {
    if (!pingToast || !text) return;
    pingText.textContent = text;
    pingToast.hidden = false;
    clearTimeout(pingTimer);
    pingTimer = setTimeout(hidePing, 30000);
    // kalau lagi idle → Noir langsung menyapa via suara; kalau sibuk, cukup toast
    if (state === 'idle' && !speaking) speakText(text);
  }
  function hidePing() { if (pingToast) pingToast.hidden = true; clearTimeout(pingTimer); }
  if (pingToast) pingToast.addEventListener('click', () => { const t = pingText.textContent; hidePing(); if (t) speakText(t); });

  /* ---------- STT (Web Speech API) ---------- */
  function startListening() {
    const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SR) { setState('idle', 'browser tidak support speech recognition — pakai Chrome'); return; }
    stopRecog();
    recog = new SR();
    recog.lang = 'id-ID';
    recog.interimResults = true;
    recog.maxAlternatives = 1;
    let finalText = '';
    setState('listening', 'mendengarkan…');
    recog.onresult = (e) => {
      let interim = '';
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const r = e.results[i];
        if (r.isFinal) finalText += r[0].transcript;
        else interim += r[0].transcript;
      }
      if (interim) statusEl.textContent = '\u201c' + interim + '\u201d';
    };
    recog.onerror = (e) => {
      if (e.error === 'not-allowed' || e.error === 'service-not-allowed')
        statusEl.textContent = 'izin mic ditolak';
      finishListen(finalText);
    };
    recog.onend = () => finishListen(finalText);
    try { recog.start(); } catch (e) { finishListen(''); }
  }
  function stopRecog() { try { recog && recog.abort(); } catch (e) {} recog = null; }
  function finishListen(text) {
    stopRecog();
    text = (text || '').trim();
    if (!text) { setState('idle', 'ketuk untuk bicara'); return; }
    setState('thinking', '\u201c' + text + '\u201d');
    send({ type: 'chat', text: text });
  }

  /* ---------- tap avatar ---------- */
  function onTap() {
    if (speaking) { // interupsi: potong omongan Noir
      speechSynthesis.cancel(); speaking = false; speakQueue = [];
      send({ type: 'interrupt' });
      setState('idle', 'ketuk untuk bicara');
      return;
    }
    if (state === 'listening') { stopRecog(); setState('idle', 'ketuk untuk bicara'); return; }
    if (state === 'thinking') return;
    startListening();
  }
  avatar.addEventListener('click', onTap);
  avatar.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') onTap(); });

  connect();
})();
