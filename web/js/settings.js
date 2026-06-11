'use strict';

// settings.js — read/write app settings from localStorage.

window.Settings = (function() {
  const KEY = 'mininext.settings';

  const defaults = {
    endpoint: '',         // server base URL; empty = same origin
    authToken: '',
    imageQuality: 'medium',
    imageFormat: 'jpeg',
    compression: 'server-default',
    deviceProfile: 'phone-modern',
    pageFormat: 'minidom-text',  // 'minidom-text' or 'mbpf'
    renderingProfile: 'box',     // 'box' (full DOM) or 'flow' (linearized)
    adBlockEnabled: true,   // on by default
  };

  function get() {
    try {
      const raw = localStorage.getItem(KEY);
      if (raw) return Object.assign({}, defaults, JSON.parse(raw));
    } catch(e) {}
    return Object.assign({}, defaults);
  }

  function save(s) {
    try {
      localStorage.setItem(KEY, JSON.stringify(s));
    } catch(e) {}
  }

  function apiBase() {
    const s = get();
    if (s.endpoint) {
      return s.endpoint.replace(/\/+$/, '');
    }
    return '';
  }

  function authHeaders() {
    const s = get();
    const headers = { 'Content-Type': 'application/json' };
    if (s.authToken) headers['Authorization'] = 'Bearer ' + s.authToken;
    return headers;
  }

  return { get, save, apiBase, authHeaders };
})();
