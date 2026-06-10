'use strict';

// api.js — Promise-based wrappers for all MiniNext REST endpoints.

window.MiniAPI = (function() {

  async function request(method, path, body, binary) {
    const base = Settings.apiBase();
    const headers = Settings.authHeaders();
    if (binary) {
      delete headers['Content-Type'];
      headers['Accept'] = 'application/x-mbpf';
    }

    const opts = {
      method,
      headers,
    };
    if (body !== undefined) {
      opts.body = JSON.stringify(body);
    }

    const res = await fetch(base + path, opts);
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.error || msg; } catch(e) {}
      throw new Error(msg);
    }

    if (binary) return res.arrayBuffer();
    return res.json();
  }

  function createSession(deviceProfile) {
    return request('POST', '/api/v1/sessions', {
      device_profile: deviceProfile || 'phone-modern',
      capabilities: {
        page_formats: ['minidom-text', 'mbpf'],
        compression: ['gzip', 'brotli'],
        image_formats: ['jpeg', 'webp', 'png', 'gif'],
        rendering_profiles: ['box', 'flow'],
      }
    });
  }

  function deleteSession(sessionID) {
    return request('DELETE', `/api/v1/sessions/${sessionID}`, undefined);
  }

  function sleepSession(sessionID) {
    return request('POST', `/api/v1/sessions/${sessionID}/sleep`, {});
  }

  function resumeSession(sessionID) {
    return request('POST', `/api/v1/sessions/${sessionID}/resume`, {});
  }

  function createTab(sessionID, url) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs`, { url });
  }

  function navigate(sessionID, tabID, url) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs/${tabID}/navigate`, { url });
  }

  async function getSnapshot(sessionID, tabID) {
    const s = Settings.get();
    const base = Settings.apiBase();
    const headers = Settings.authHeaders();

    const useMBPF = s.pageFormat === 'mbpf';
    if (useMBPF) {
      headers['Accept'] = 'application/x-mbpf';
    } else {
      headers['Accept'] = 'application/minidom+json';
    }
    headers['Accept-Encoding'] = 'gzip, br';

    const res = await fetch(
      `${base}/api/v1/sessions/${sessionID}/tabs/${tabID}/snapshot`,
      { headers }
    );
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.error || msg; } catch(e) {}
      throw new Error(msg);
    }

    const snapshotID = parseInt(res.headers.get('X-Snapshot-Id') || '0', 10);

    if (useMBPF) {
      const buf = await res.arrayBuffer();
      const snap = MBPFDecoder.decode(buf);
      snap.snapshot_id = snapshotID;
      return snap;
    } else {
      const snap = await res.json();
      snap.snapshot_id = snapshotID;
      return snap;
    }
  }

  function interact(sessionID, tabID, snapshotID, event) {
    return request('POST',
      `/api/v1/sessions/${sessionID}/tabs/${tabID}/interact`,
      { snapshot_id: snapshotID, event }
    );
  }

  function getResource(sessionID, tabID, resourceID) {
    const base = Settings.apiBase();
    return `${base}/api/v1/sessions/${sessionID}/tabs/${tabID}/resources/${resourceID}`;
  }

  return {
    createSession,
    deleteSession,
    sleepSession,
    resumeSession,
    createTab,
    navigate,
    getSnapshot,
    interact,
    getResource,
  };
})();
