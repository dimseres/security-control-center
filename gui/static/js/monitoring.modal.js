(() => {
  const els = {};
  const modalState = { editingId: null, submitting: false };
  const URL_TYPES = new Set(['http', 'http_keyword', 'http_json', 'postgres', 'grpc_keyword']);
  const HOST_PORT_TYPES = new Set(['tcp', 'ping', 'dns', 'docker', 'steam', 'gamedig', 'mqtt', 'kafka_producer', 'mssql', 'mysql', 'mongodb', 'radius', 'redis', 'tailscale_ping']);
  const HTTP_TYPES = new Set(['http', 'http_keyword', 'http_json']);

  function bindModal() {
    els.modal = document.getElementById('monitor-modal');
    els.title = document.getElementById('monitor-modal-title');
    els.alert = document.getElementById('monitor-modal-alert');
    els.form = document.getElementById('monitor-form');
    els.save = document.getElementById('monitor-save');
    els.type = document.getElementById('monitor-type');
    els.name = document.getElementById('monitor-name');
    els.url = document.getElementById('monitor-url');
    els.host = document.getElementById('monitor-host');
    els.port = document.getElementById('monitor-port');
    els.interval = document.getElementById('monitor-interval');
    els.timeout = document.getElementById('monitor-timeout');
    els.retries = document.getElementById('monitor-retries');
    els.retryInterval = document.getElementById('monitor-retry-interval');
    els.method = document.getElementById('monitor-method');
    els.allowedStatus = document.getElementById('monitor-allowed-status');
    els.headers = document.getElementById('monitor-headers');
    els.body = document.getElementById('monitor-body');
    els.bodyType = document.getElementById('monitor-body-type');
    els.tags = document.getElementById('monitor-tags');
    els.tagsHint = document.querySelector('[data-tag-hint="monitor-tags"]');
    els.autoIncident = document.getElementById('monitor-auto-incident');
    els.autoIncidentRow = document.getElementById('monitor-auto-incident-row');
    els.autoTaskOnDown = document.getElementById('monitor-auto-task-on-down');
    els.incidentSeverity = document.getElementById('monitor-incident-severity');
    els.notifyTLS = document.getElementById('monitor-notify-tls');
    els.ignoreTLS = document.getElementById('monitor-ignore-tls');
    els.notifications = document.getElementById('monitor-notifications-list');
    document.querySelectorAll('[data-close="#monitor-modal"]').forEach(btn => {
      btn.addEventListener('click', () => {
        if (els.modal) els.modal.hidden = true;
      });
    });

    if (els.type) {
      els.type.addEventListener('change', () => {
        adaptTargetFieldsForType(els.type.value);
        toggleTypeFields(els.type.value);
      });
    }
    if (els.save) {
      els.save.addEventListener('click', submitForm);
    }
    if (els.tags && DocsPage?.enhanceMultiSelects) {
      DocsPage.enhanceMultiSelects([els.tags.id]);
    }
    if (els.tags && MonitoringPage.bindTagHint) {
      MonitoringPage.bindTagHint(els.tags, els.tagsHint);
    }
    if (els.autoIncident) {
      els.autoIncident.addEventListener('change', () => applyIncidentControlState());
    }
    toggleTypeFields('http');
  }

  async function openMonitorModal(monitor) {
    if (!els.modal) return;
    modalState.submitting = false;
    modalState.editingId = monitor?.id || null;
    MonitoringPage.hideAlert(els.alert);
    els.form?.reset();
    setSubmitState(false);
    fillTagOptions(monitor?.tags || []);
    if (monitor) {
      els.title.textContent = MonitoringPage.t('monitoring.modal.editTitle');
      els.type.value = monitor.type || 'http';
      els.name.value = monitor.name || '';
      els.url.value = monitor.url || '';
      els.host.value = monitor.host || '';
      els.port.value = monitor.port || '';
      els.interval.value = monitor.interval_sec || '';
      els.timeout.value = monitor.timeout_sec || '';
      els.retries.value = monitor.retries || 0;
      els.retryInterval.value = monitor.retry_interval_sec || '';
      els.method.value = monitor.method || 'GET';
      els.allowedStatus.value = (monitor.allowed_status || []).join(', ');
      els.headers.value = JSON.stringify(monitor.headers || {}, null, 2);
      els.body.value = monitor.request_body || '';
      els.bodyType.value = monitor.request_body_type || 'none';
      setSelectedOptions(els.tags, monitor.tags || []);
      if (els.tags) {
        els.tags.dispatchEvent(new Event('change', { bubbles: true }));
      }
      if (els.autoIncident) {
        els.autoIncident.checked = !!monitor.auto_incident;
      }
      if (els.autoTaskOnDown) {
        els.autoTaskOnDown.checked = monitor.auto_task_on_down !== false;
      }
      if (els.notifyTLS) {
        els.notifyTLS.checked = monitor.notify_tls_expiring !== false;
      }
      if (els.ignoreTLS) {
        els.ignoreTLS.checked = !!monitor.ignore_tls_errors;
      }
      if (els.incidentSeverity) {
        els.incidentSeverity.value = monitor.incident_severity || 'low';
      }
    } else {
      els.title.textContent = MonitoringPage.t('monitoring.modal.createTitle');
      const defaults = MonitoringPage.state.settings || {};
      els.type.value = 'http';
      els.method.value = 'GET';
      els.bodyType.value = 'none';
      els.interval.value = defaults.default_interval_sec || 30;
      els.timeout.value = defaults.default_timeout_sec || 20;
      els.retryInterval.value = defaults.default_retry_interval_sec || 30;
      els.retries.value = defaults.default_retries ?? 2;
      els.allowedStatus.value = '200-299';
      if (els.autoIncident) {
        els.autoIncident.checked = false;
      }
      if (els.autoTaskOnDown) {
        els.autoTaskOnDown.checked = false;
      }
      if (els.notifyTLS) {
        els.notifyTLS.checked = true;
      }
      if (els.ignoreTLS) {
        els.ignoreTLS.checked = false;
      }
      if (els.incidentSeverity) {
        els.incidentSeverity.value = 'low';
      }
    }
    await renderNotificationLinks(monitor?.id || null);
    toggleIncidentFields();
    applyIncidentControlState();
    toggleTypeFields(els.type.value);
    els.modal.hidden = false;
  }

  async function submitForm() {
    if (modalState.submitting) return;
    MonitoringPage.hideAlert(els.alert);
    const payload = buildPayload();
    if (!payload) return;
    modalState.submitting = true;
    setSubmitState(true);
    try {
      let id = modalState.editingId;
      if (modalState.editingId) {
        await Api.put(`/api/monitoring/monitors/${modalState.editingId}`, payload);
      } else {
        const created = await Api.post('/api/monitoring/monitors', payload);
        id = created?.id || created?.ID || null;
        if (id) {
          MonitoringPage.state.selectedId = id;
        }
      }
      if (id && MonitoringPage.hasPermission('monitoring.notifications.manage')) {
        await saveMonitorNotifications(id);
      }
      if (id && MonitoringPage.hasPermission('monitoring.view')) {
        await waitMonitorFirstCheck(id);
      }
      els.modal.hidden = true;
      modalState.editingId = null;
      await MonitoringPage.loadMonitors?.();
      await MonitoringPage.refreshSLA?.();
      await MonitoringPage.refreshMaintenanceList?.();
      MonitoringPage.refreshEventsFilters?.();
      MonitoringPage.refreshEventsCenter?.();
      MonitoringPage.refreshCerts?.();
    } catch (err) {
      MonitoringPage.showAlert(els.alert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
    } finally {
      modalState.submitting = false;
      setSubmitState(false);
    }
  }

  function setSubmitState(submitting) {
    if (!els.save) return;
    els.save.disabled = !!submitting;
    els.save.classList.toggle('disabled', !!submitting);
  }

  async function waitMonitorFirstCheck(id) {
    const timeoutMs = 7000;
    const stepMs = 450;
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      try {
        const res = await Api.get(`/api/monitoring/monitors/${id}/state`);
        const item = res?.item || null;
        if (item && item.last_checked_at) return;
      } catch (_) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, stepMs));
    }
  }

  function buildPayload() {
    const type = els.type.value;
    const headers = parseHeaders();
    if (headers === null) return null;
    const payload = {
      type,
      name: els.name.value.trim(),
      interval_sec: parseInt(els.interval.value, 10) || 0,
      timeout_sec: parseInt(els.timeout.value, 10) || 0,
      retries: parseInt(els.retries.value, 10) || 0,
      retry_interval_sec: parseInt(els.retryInterval.value, 10) || 0,
      method: els.method.value,
      allowed_status: splitList(els.allowedStatus.value),
      request_body: els.body.value || '',
      request_body_type: els.bodyType.value,
      headers: headers,
      tags: getSelectedOptions(els.tags),
    };
    if (els.autoIncident && MonitoringPage.hasPermission('monitoring.incidents.link')) {
      payload.auto_incident = !!els.autoIncident.checked;
      payload.incident_severity = els.incidentSeverity?.value || 'low';
    }
    if (els.autoTaskOnDown) {
      payload.auto_task_on_down = !!els.autoTaskOnDown.checked;
    }
    if (els.notifyTLS) {
      payload.notify_tls_expiring = !!els.notifyTLS.checked;
    }
    if (els.ignoreTLS) {
      payload.ignore_tls_errors = !!els.ignoreTLS.checked;
    }
    if (URL_TYPES.has(type)) {
      payload.url = els.url.value.trim();
    } else if (HOST_PORT_TYPES.has(type)) {
      payload.host = els.host.value.trim();
      payload.port = parseInt(els.port.value, 10) || 0;
    } else if (type === 'push') {
      payload.request_body = (els.body.value || '').trim();
      payload.request_body_type = 'none';
    }
    return payload;
  }

  function parseHeaders() {
    const raw = (els.headers.value || '').trim();
    if (!raw) return {};
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('invalid');
      }
      return parsed;
    } catch (err) {
      MonitoringPage.showAlert(els.alert, MonitoringPage.t('monitoring.error.invalidHeaders'), false);
      return null;
    }
  }

  function fillTagOptions(selected) {
    if (!els.tags) return;
    els.tags.innerHTML = '';
    const existing = new Set(selected || []);
    if (typeof TagDirectory !== 'undefined' && TagDirectory.all) {
      TagDirectory.all().forEach(tag => existing.add(tag.code || tag));
    }
    Array.from(existing).sort().forEach(tag => {
      const opt = document.createElement('option');
      opt.value = tag;
      opt.textContent = (typeof TagDirectory !== 'undefined' && TagDirectory.label)
        ? (TagDirectory.label(tag) || tag)
        : tag;
      opt.dataset.label = opt.textContent;
      els.tags.appendChild(opt);
    });
    if (MonitoringPage.bindTagHint) {
      MonitoringPage.bindTagHint(els.tags, els.tagsHint);
    }
  }

  function toggleTypeFields(type) {
    const kind = (type || '').toLowerCase();
    const isHTTP = HTTP_TYPES.has(kind);
    const usesURL = URL_TYPES.has(kind);
    const usesHostPort = HOST_PORT_TYPES.has(kind);
    const hasHTTPRequest = isHTTP;
    const isPush = kind === 'push';
    const isGRPC = kind === 'grpc_keyword';
    const bodyTypeField = els.bodyType ? els.bodyType.closest('.form-field') : null;
    const bodyLabel = document.querySelector('#monitor-body-field label');

    document.getElementById('monitor-url-field').hidden = !usesURL;
    document.getElementById('monitor-host-field').hidden = !usesHostPort;
    document.getElementById('monitor-port-field').hidden = !usesHostPort || kind === 'dns' || kind === 'tailscale_ping';
    document.getElementById('monitor-method-field').hidden = !(hasHTTPRequest || isGRPC);
    document.getElementById('monitor-status-field').hidden = !hasHTTPRequest;
    document.getElementById('monitor-headers-field').hidden = !(hasHTTPRequest || isGRPC);
    if (bodyTypeField) bodyTypeField.hidden = !hasHTTPRequest || kind === 'http_keyword' || isPush || isGRPC;
    document.getElementById('monitor-body-field').hidden = !(hasHTTPRequest || kind === 'dns' || isPush);

    if (kind === 'http_keyword') {
      if (bodyLabel) bodyLabel.textContent = MonitoringPage.t('monitoring.field.expectedWord');
      if (els.bodyType) els.bodyType.value = 'none';
    } else if (isPush) {
      if (bodyLabel) bodyLabel.textContent = MonitoringPage.t('monitoring.field.pushToken');
      if (els.bodyType) els.bodyType.value = 'none';
      if (els.body && !els.body.value.trim()) {
        els.body.value = randomPushToken();
      }
    } else if (kind === 'dns') {
      if (bodyLabel) bodyLabel.textContent = MonitoringPage.t('monitoring.field.dnsExpected');
    } else if (isGRPC) {
      if (bodyLabel) bodyLabel.textContent = MonitoringPage.t('monitoring.field.body');
      if (els.method) els.method.value = 'GET';
    } else if (kind === 'http_json' || kind === 'http') {
      if (bodyLabel) bodyLabel.textContent = MonitoringPage.t('monitoring.field.body');
    }
    if (els.notifyTLS) els.notifyTLS.closest('.form-field').hidden = !isHTTP;
    if (els.ignoreTLS) els.ignoreTLS.closest('.form-field').hidden = !isHTTP;
  }

  function adaptTargetFieldsForType(nextType) {
    const kind = (nextType || '').toLowerCase();
    const toURL = URL_TYPES.has(kind);
    const toHostPort = HOST_PORT_TYPES.has(kind);
    if (toHostPort) {
      const urlValue = (els.url?.value || '').trim();
      if (urlValue) {
        try {
          const u = new URL(urlValue);
          if (els.host && !els.host.value.trim()) {
            els.host.value = u.hostname || '';
          }
          if (els.port && !els.port.value.trim()) {
            if (u.port) {
              els.port.value = u.port;
            } else if (u.protocol === 'https:') {
              els.port.value = '443';
            } else if (u.protocol === 'http:') {
              els.port.value = '80';
            }
          }
        } catch (_) {
          // ignore parse failures and keep entered values as-is
        }
      }
    }
    if (toURL) {
      const currentURL = (els.url?.value || '').trim();
      const host = (els.host?.value || '').trim();
      const port = (els.port?.value || '').trim();
      if (!currentURL && host) {
        const scheme = (port === '443' || kind === 'grpc_keyword') ? (kind === 'grpc_keyword' ? 'grpcs' : 'https') : (kind === 'grpc_keyword' ? 'grpc' : 'http');
        const hostPort = port ? `${host}:${port}` : host;
        if (els.url) {
          els.url.value = `${scheme}://${hostPort}`;
        }
      }
    }
  }

  function toggleIncidentFields() {
    const canLink = MonitoringPage.hasPermission('monitoring.incidents.link');
    if (els.autoIncidentRow) {
      els.autoIncidentRow.hidden = !canLink;
    }
    if (els.autoIncident) {
      els.autoIncident.disabled = !canLink;
    }
    if (els.incidentSeverity) {
      const field = els.incidentSeverity.closest('.form-field');
      if (field) field.hidden = !canLink;
      els.incidentSeverity.disabled = !canLink;
    }
  }

  function applyIncidentControlState() {
    if (!els.autoIncident || !els.incidentSeverity) return;
    const enabled = MonitoringPage.hasPermission('monitoring.incidents.link');
    els.incidentSeverity.disabled = !enabled;
  }

  async function renderNotificationLinks(monitorId) {
    if (!els.notifications) return;
    const canManage = MonitoringPage.hasPermission('monitoring.notifications.manage');
    const canView = canManage || MonitoringPage.hasPermission('monitoring.notifications.view');
    if (!canView) {
      els.notifications.closest('.form-field').hidden = true;
      return;
    }
    els.notifications.closest('.form-field').hidden = false;
    const channels = await MonitoringPage.ensureNotificationChannels?.();
    let activeIds = [];
    let linkMap = new Map();
    let hasExplicitBindings = false;
    if (monitorId && canView) {
      try {
        const res = await Api.get(`/api/monitoring/monitors/${monitorId}/notifications`);
        const items = Array.isArray(res.items) ? res.items : [];
        hasExplicitBindings = items.length > 0;
        items.forEach((item) => {
          const id = Number(item.notification_channel_id || 0);
          if (!id) return;
          linkMap.set(id, !!item.enabled);
        });
        activeIds = items.filter(i => i.enabled).map(i => i.notification_channel_id);
      } catch (_) {
        activeIds = [];
      }
    }
    els.notifications.innerHTML = '';
    if (!channels || !channels.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = MonitoringPage.t('monitoring.notifications.empty');
      els.notifications.appendChild(empty);
      return;
    }
    channels.forEach(ch => {
      const row = document.createElement('label');
      row.className = 'tag-option';
      row.innerHTML = `
        <input type="checkbox" value="${ch.id}">
        <span>${escapeHtml(ch.name)} (${escapeHtml(ch.type)})</span>`;
      const input = row.querySelector('input');
      if (input) {
        if (linkMap.has(ch.id)) {
          input.checked = !!linkMap.get(ch.id);
        } else if (!hasExplicitBindings && !!ch.is_default) {
          // No monitor-level bindings yet: reflect default channels as preselected.
          input.checked = true;
        } else {
          input.checked = activeIds.includes(ch.id);
        }
        input.disabled = !canManage;
      }
      els.notifications.appendChild(row);
    });
  }

  async function saveMonitorNotifications(monitorId) {
    if (!els.notifications) return;
    const checkboxes = Array.from(els.notifications.querySelectorAll('input[type="checkbox"]'));
    const items = checkboxes
      .map((input) => ({
        notification_channel_id: parseInt(input.value, 10) || 0,
        enabled: !!input.checked,
      }))
      .filter((item) => item.notification_channel_id > 0);
    await Api.put(`/api/monitoring/monitors/${monitorId}/notifications`, { items });
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function splitList(raw) {
    if (!raw) return [];
    return raw.split(',').map(v => v.trim()).filter(Boolean);
  }

  function randomPushToken() {
    const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    let out = '';
    for (let i = 0; i < 32; i += 1) {
      out += chars[Math.floor(Math.random() * chars.length)];
    }
    return out;
  }

  function getSelectedOptions(select) {
    if (!select) return [];
    return Array.from(select.selectedOptions).map(o => o.value);
  }

  function setSelectedOptions(select, values) {
    if (!select) return;
    const set = new Set(values || []);
    Array.from(select.options).forEach(opt => {
      opt.selected = set.has(opt.value);
    });
  }

  if (typeof MonitoringPage !== 'undefined') {
    MonitoringPage.bindModal = bindModal;
    MonitoringPage.openMonitorModal = openMonitorModal;
  }
})();
