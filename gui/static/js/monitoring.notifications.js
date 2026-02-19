(() => {
  const els = {};
  const modalState = { editingId: null, tokenVisible: false, originalToken: '' };

  function bindNotifications() {
    const canView = MonitoringPage.hasPermission('monitoring.notifications.view')
      || MonitoringPage.hasPermission('monitoring.notifications.manage');
    if (!canView) {
      const tab = document.getElementById('monitoring-tab-notify');
      if (tab) tab.hidden = true;
      return;
    }
    els.list = document.getElementById('monitoring-notify-list');
    els.deliveryList = document.getElementById('monitoring-notify-delivery-list');
    els.deliveryRefresh = document.getElementById('monitoring-notify-delivery-refresh');
    els.alert = document.getElementById('monitoring-notify-alert');
    els.newBtn = document.getElementById('monitoring-notify-new');
    bindModal();
    if (els.newBtn) {
      els.newBtn.addEventListener('click', () => openModal());
      if (!MonitoringPage.hasPermission('monitoring.notifications.manage')) {
        els.newBtn.disabled = true;
      }
    }
    if (els.deliveryRefresh) {
      els.deliveryRefresh.addEventListener('click', () => loadDeliveries());
    }
    loadChannels();
    loadDeliveries();
  }

  function bindModal() {
    els.modal = document.getElementById('notification-modal');
    els.modalTitle = document.getElementById('notification-modal-title');
    els.modalAlert = document.getElementById('notification-modal-alert');
    els.modalSave = document.getElementById('notification-save');
    els.modalForm = document.getElementById('notification-form');
    els.type = document.getElementById('notification-type');
    els.name = document.getElementById('notification-name');
    els.token = document.getElementById('notification-token');
    els.tokenToggle = document.getElementById('notification-token-toggle');
    els.chatId = document.getElementById('notification-chat-id');
    els.threadId = document.getElementById('notification-thread-id');
    els.template = document.getElementById('notification-template');
    els.quietEnabled = document.getElementById('notification-quiet-enabled');
    els.quietStart = document.getElementById('notification-quiet-start');
    els.quietEnd = document.getElementById('notification-quiet-end');
    els.quietTz = document.getElementById('notification-quiet-tz');
    els.silent = document.getElementById('notification-silent');
    els.protect = document.getElementById('notification-protect');
    els.default = document.getElementById('notification-default');
    els.active = document.getElementById('notification-active');
    els.applyAll = document.getElementById('notification-apply-all');
    els.applyAllRow = document.getElementById('notification-apply-all-row');
    document.querySelectorAll('[data-close="#notification-modal"]').forEach(btn => {
      btn.addEventListener('click', () => {
        if (els.modal) els.modal.hidden = true;
      });
    });
    if (els.modalSave) {
      els.modalSave.addEventListener('click', saveChannel);
    }
    if (els.tokenToggle && els.token) {
      els.tokenToggle.addEventListener('click', () => toggleTokenVisibility());
    }
  }

  async function loadChannels() {
    try {
      const res = await Api.get('/api/monitoring/notifications');
      MonitoringPage.state.notificationChannels = res.items || [];
      renderList(MonitoringPage.state.notificationChannels);
    } catch (err) {
      console.error('notifications', err);
    }
  }

  function renderList(items) {
    if (!els.list) return;
    const canManage = MonitoringPage.hasPermission('monitoring.notifications.manage');
    if (!items.length) {
      els.list.innerHTML = `<div class="empty-state">${MonitoringPage.t('monitoring.notifications.empty')}</div>`;
      return;
    }
    const rows = items.map(item => {
      const tokenPreview = maskToken(item.telegram_bot_token || '');
      const status = item.is_active ? MonitoringPage.t('common.active') : MonitoringPage.t('common.disabled');
      const defaultBadge = item.is_default ? `<span class="badge">${MonitoringPage.t('monitoring.notifications.default')}</span>` : '';
      return `
        <tr data-id="${item.id}">
          <td>
            <div class="cell-title">${escapeHtml(item.name || '')}</div>
            <div class="cell-subtitle">${escapeHtml(tokenPreview)}</div>
          </td>
          <td>${escapeHtml(item.type || '')}</td>
          <td>${escapeHtml(item.telegram_chat_id || '')}</td>
          <td>${defaultBadge}</td>
          <td>${escapeHtml(status)}</td>
          <td>
            <button class="btn ghost notify-edit"${canManage ? '' : ' disabled'}>${MonitoringPage.t('common.edit')}</button>
            <button class="btn ghost danger notify-delete"${canManage ? '' : ' disabled'}>${MonitoringPage.t('common.delete')}</button>
            <button class="btn ghost notify-test"${canManage ? '' : ' disabled'}>${MonitoringPage.t('monitoring.notifications.test')}</button>
          </td>
        </tr>`;
    }).join('');
    els.list.innerHTML = `
      <table class="data-table compact">
        <thead>
          <tr>
            <th>${MonitoringPage.t('monitoring.notifications.name')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.type')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.chat')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.default')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.status')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.actions')}</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>`;
    els.list.querySelectorAll('.notify-edit').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const id = parseInt(e.target.closest('tr')?.dataset.id || '0', 10);
        const item = items.find(ch => ch.id === id);
        if (item) openModal(item);
      });
    });
    els.list.querySelectorAll('.notify-delete').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const id = parseInt(e.target.closest('tr')?.dataset.id || '0', 10);
        if (!id || !confirm(MonitoringPage.t('monitoring.notifications.confirmDelete'))) return;
        try {
          await Api.del(`/api/monitoring/notifications/${id}`);
          await loadChannels();
        } catch (err) {
          MonitoringPage.showAlert(els.alert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
        }
      });
    });
    els.list.querySelectorAll('.notify-test').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const id = parseInt(e.target.closest('tr')?.dataset.id || '0', 10);
        if (!id) return;
        try {
          await Api.post(`/api/monitoring/notifications/${id}/test`);
          MonitoringPage.showAlert(els.alert, MonitoringPage.t('monitoring.notifications.testSent'), true);
        } catch (err) {
          MonitoringPage.showAlert(els.alert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
        }
      });
    });
  }

  function openModal(channel) {
    if (!els.modal) return;
    modalState.editingId = channel?.id || null;
    modalState.tokenVisible = false;
    modalState.originalToken = channel?.telegram_bot_token || '';
    MonitoringPage.hideAlert(els.modalAlert);
    els.modalForm?.reset();
    if (els.applyAllRow) els.applyAllRow.hidden = !!channel;
    if (channel) {
      els.modalTitle.textContent = MonitoringPage.t('monitoring.notifications.editTitle');
      els.type.value = channel.type || 'telegram';
      els.name.value = channel.name || '';
      els.token.value = channel.telegram_bot_token || '';
      els.chatId.value = channel.telegram_chat_id || '';
      els.threadId.value = channel.telegram_thread_id || '';
      els.template.value = channel.template_text || '';
      els.quietEnabled.checked = !!channel.quiet_hours_enabled;
      els.quietStart.value = channel.quiet_hours_start || '';
      els.quietEnd.value = channel.quiet_hours_end || '';
      els.quietTz.value = channel.quiet_hours_tz || defaultQuietTimezone();
      els.silent.checked = !!channel.silent;
      els.protect.checked = !!channel.protect_content;
      els.default.checked = !!channel.is_default;
      els.active.checked = !!channel.is_active;
    } else {
      els.modalTitle.textContent = MonitoringPage.t('monitoring.notifications.createTitle');
      els.type.value = 'telegram';
      els.template.value = '{message}';
      els.quietEnabled.checked = false;
      els.quietStart.value = '';
      els.quietEnd.value = '';
      els.quietTz.value = defaultQuietTimezone();
      els.default.checked = false;
      els.active.checked = true;
    }
    if (els.token) {
      els.token.type = 'password';
    }
    els.modal.hidden = false;
  }

  function defaultQuietTimezone() {
    if (typeof AppTime !== 'undefined' && AppTime.getTimeZone) {
      return AppTime.getTimeZone() || 'UTC';
    }
    if (typeof Preferences !== 'undefined' && Preferences.load) {
      return Preferences.load().timeZone || 'UTC';
    }
    return 'UTC';
  }

  async function saveChannel() {
    if (!MonitoringPage.hasPermission('monitoring.notifications.manage')) return;
    MonitoringPage.hideAlert(els.modalAlert);
    const payload = buildPayload();
    if (!payload) return;
    try {
      if (modalState.editingId) {
        await Api.put(`/api/monitoring/notifications/${modalState.editingId}`, payload);
      } else {
        await Api.post('/api/monitoring/notifications', payload);
      }
      els.modal.hidden = true;
      modalState.editingId = null;
      await loadChannels();
    } catch (err) {
      MonitoringPage.showAlert(els.modalAlert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
    }
  }

  function buildPayload() {
    const name = (els.name.value || '').trim();
    let token = (els.token.value || '').trim();
    if (modalState.editingId && token) {
      if (token.includes('*') || token === (modalState.originalToken || '').trim()) {
        token = '';
      }
    }
    const chatId = (els.chatId.value || '').trim();
    if (!name) {
      MonitoringPage.showAlert(els.modalAlert, MonitoringPage.t('monitoring.notifications.nameRequired'), false);
      return null;
    }
    if (!modalState.editingId && (!token || !chatId)) {
      MonitoringPage.showAlert(els.modalAlert, MonitoringPage.t('monitoring.notifications.telegramRequired'), false);
      return null;
    }
    return {
      type: 'telegram',
      name,
      telegram_bot_token: token,
      telegram_chat_id: chatId,
      telegram_thread_id: els.threadId.value ? parseInt(els.threadId.value, 10) || null : null,
      template_text: (els.template.value || '').trim(),
      quiet_hours_enabled: !!els.quietEnabled.checked,
      quiet_hours_start: (els.quietStart.value || '').trim(),
      quiet_hours_end: (els.quietEnd.value || '').trim(),
      quiet_hours_tz: (els.quietTz.value || '').trim(),
      silent: !!els.silent.checked,
      protect_content: !!els.protect.checked,
      is_default: !!els.default.checked,
      is_active: !!els.active.checked,
      apply_to_all: !!els.applyAll?.checked
    };
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  async function toggleTokenVisibility() {
    if (!els.token) return;
    const nextVisible = !modalState.tokenVisible;
    if (nextVisible && modalState.editingId && (els.token.value || '').includes('*')) {
      try {
        const res = await Api.get(`/api/monitoring/notifications/${modalState.editingId}/token`);
        const raw = (res?.telegram_bot_token || '').toString();
        els.token.value = raw;
        modalState.originalToken = raw.trim();
      } catch (err) {
        MonitoringPage.showAlert(els.modalAlert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
        return;
      }
    }
    modalState.tokenVisible = nextVisible;
    els.token.type = modalState.tokenVisible ? 'text' : 'password';
    if (els.tokenToggle) {
      els.tokenToggle.classList.toggle('active', modalState.tokenVisible);
    }
  }

  function maskToken(token) {
    const raw = (token || '').trim();
    if (!raw) return '';
    if (raw.length <= 8) return '******';
    return `${raw.slice(0, 4)}...${raw.slice(-4)}`;
  }

  async function ensureNotificationChannels(force) {
    if (!force && Array.isArray(MonitoringPage.state.notificationChannels) && MonitoringPage.state.notificationChannels.length) {
      return MonitoringPage.state.notificationChannels;
    }
    await loadChannels();
    return MonitoringPage.state.notificationChannels || [];
  }

  async function loadDeliveries() {
    if (!els.deliveryList) return;
    try {
      const res = await Api.get('/api/monitoring/notifications/deliveries?limit=100');
      renderDeliveries(res.items || []);
    } catch (err) {
      console.error('deliveries', err);
    }
  }

  function renderDeliveries(items) {
    if (!els.deliveryList) return;
    const canManage = MonitoringPage.hasPermission('monitoring.notifications.manage');
    if (!items.length) {
      els.deliveryList.innerHTML = `<div class="empty-state">${MonitoringPage.t('monitoring.notifications.deliveryEmpty')}</div>`;
      return;
    }
    const rows = items.map((item) => {
      const ack = item.acknowledged_at
        ? `${MonitoringPage.t('monitoring.notifications.acknowledged')} ${MonitoringPage.formatDate(item.acknowledged_at)}`
        : MonitoringPage.t('monitoring.notifications.notAcknowledged');
      const ackBtn = !item.acknowledged_at && canManage
        ? `<button class="btn ghost notify-ack" data-id="${item.id}">${MonitoringPage.t('monitoring.notifications.ack')}</button>`
        : '';
      return `
        <tr>
          <td>${MonitoringPage.formatDate(item.created_at)}</td>
          <td>${escapeHtml(item.event_type || '')}</td>
          <td>${escapeHtml(item.status || '')}</td>
          <td>${escapeHtml(item.error || '')}</td>
          <td>${escapeHtml(item.body_preview || '')}</td>
          <td>${escapeHtml(ack)}</td>
          <td>${ackBtn}</td>
        </tr>`;
    }).join('');
    els.deliveryList.innerHTML = `
      <table class="data-table compact">
        <thead>
          <tr>
            <th>${MonitoringPage.t('common.time')}</th>
            <th>${MonitoringPage.t('monitoring.filter.type')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.deliveryStatus')}</th>
            <th>${MonitoringPage.t('common.error')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.deliveryMessage')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.deliveryAck')}</th>
            <th>${MonitoringPage.t('monitoring.notifications.actions')}</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>`;
    els.deliveryList.querySelectorAll('.notify-ack').forEach((btn) => {
      btn.addEventListener('click', async (e) => {
        const id = parseInt(e.currentTarget.dataset.id || '0', 10);
        if (!id) return;
        try {
          await Api.post(`/api/monitoring/notifications/deliveries/${id}/ack`);
          await loadDeliveries();
        } catch (err) {
          MonitoringPage.showAlert(els.alert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
        }
      });
    });
  }

  if (typeof MonitoringPage !== 'undefined') {
    MonitoringPage.bindNotifications = bindNotifications;
    MonitoringPage.ensureNotificationChannels = ensureNotificationChannels;
    MonitoringPage.loadNotificationDeliveries = loadDeliveries;
  }
})();
