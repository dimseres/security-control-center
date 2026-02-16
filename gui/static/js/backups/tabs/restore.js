const BackupsRestore = (() => {
  const state = {
    restoreId: 0,
    pollTimer: null,
  };

  function init() {
    stopPolling();
    state.restoreId = 0;
    bindActions();
  }

  function bindActions() {
    const dryRunBtn = document.getElementById('backups-restore-dry-run');
    const restoreBtn = document.getElementById('backups-restore-run');
    const refreshBtn = document.getElementById('backups-restore-refresh');
    const importOpenBtn = document.getElementById('backups-import-open');
    const importSubmitBtn = document.getElementById('backups-import-submit');
    const importCancelBtn = document.getElementById('backups-import-cancel');
    const importCancelTopBtn = document.getElementById('backups-import-cancel-top');
    if (dryRunBtn) dryRunBtn.addEventListener('click', () => startRestore(true));
    if (restoreBtn) restoreBtn.addEventListener('click', () => startRestore(false));
    if (refreshBtn) refreshBtn.addEventListener('click', () => load(true));
    if (importOpenBtn) importOpenBtn.addEventListener('click', openImportModal);
    if (importSubmitBtn) importSubmitBtn.addEventListener('click', submitImport);
    if (importCancelBtn) importCancelBtn.addEventListener('click', closeImportModal);
    if (importCancelTopBtn) importCancelTopBtn.addEventListener('click', closeImportModal);
  }

  async function load(showSpinner = false) {
    if (showSpinner) toggleLoading(true);
    try {
      const items = await BackupsPage.listBackups();
      renderArtifacts(items);
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      renderArtifacts([]);
    } finally {
      toggleLoading(false);
    }
  }

  function renderArtifacts(items) {
    const select = document.getElementById('backups-restore-artifact');
    if (!select) return;
    select.innerHTML = '';
    const successItems = (items || []).filter((item) => item.status === 'success');
    if (!successItems.length) {
      const option = document.createElement('option');
      option.value = '';
      option.textContent = BackupsPage.t('backups.empty.noBackups');
      select.appendChild(option);
      return;
    }
    successItems.forEach((item) => {
      const option = document.createElement('option');
      option.value = `${item.id}`;
      option.textContent = `${item.filename || `#${item.id}`} (${BackupsPage.formatDateTime(item.created_at)})`;
      select.appendChild(option);
    });
  }

  async function startRestore(dryRun) {
    const select = document.getElementById('backups-restore-artifact');
    const backupId = parseInt(select?.value || '0', 10);
    if (!backupId) {
      BackupsPage.setAlert('error', 'backups.error.notFound', BackupsPage.t('backups.empty.noBackups'));
      return;
    }
    toggleLoading(true);
    BackupsPage.setAlert('', '', '');
    try {
      const path = dryRun
        ? `/api/backups/${backupId}/restore/dry-run`
        : `/api/backups/${backupId}/restore`;
      const res = await BackupsPage.apiPost(path, {});
      state.restoreId = Number(res.restore_id || res.item?.id || 0);
      setCurrentRestore(res.item);
      BackupsPage.setAlert('success', dryRun ? 'backups.restore.dryRun.started' : 'backups.restore.started', '');
      startPolling();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
    } finally {
      toggleLoading(false);
    }
  }

  function startPolling() {
    stopPolling();
    if (!state.restoreId) return;
    pollRestoreStatus();
    state.pollTimer = setInterval(pollRestoreStatus, 1500);
  }

  function stopPolling() {
    if (!state.pollTimer) return;
    clearInterval(state.pollTimer);
    state.pollTimer = null;
  }

  async function pollRestoreStatus() {
    if (!document.getElementById('backups-page')) {
      stopPolling();
      return;
    }
    if (!state.restoreId) return;
    try {
      const res = await BackupsPage.apiGet(`/api/backups/restores/${state.restoreId}`);
      const item = res.item || null;
      setCurrentRestore(item);
      renderSteps(item?.steps || []);
      const status = item?.status;
      if (status === 'success' || status === 'failed' || status === 'canceled') {
        stopPolling();
      }
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      stopPolling();
    }
  }

  function setCurrentRestore(item) {
    const el = document.getElementById('backups-restore-current');
    if (!el) return;
    if (!item) {
      el.textContent = '';
      return;
    }
    el.textContent = `${BackupsPage.t('backups.fields.status')}: ${BackupsPage.statusLabel(item.status)} | ID: ${item.id}`;
  }

  function renderSteps(steps) {
    const list = document.getElementById('backups-restore-steps');
    if (!list) return;
    list.innerHTML = '';
    if (!Array.isArray(steps) || steps.length === 0) {
      const li = document.createElement('li');
      li.textContent = BackupsPage.t('backups.restore.noProgress');
      list.appendChild(li);
      return;
    }
    steps.forEach((step) => {
      const li = document.createElement('li');
      li.className = 'restore-step-item';
      const name = document.createElement('span');
      name.className = 'restore-step-name';
      name.textContent = BackupsPage.t(step.message_i18n_key) || step.message_i18n_key || step.name;
      li.appendChild(name);
      const status = document.createElement('span');
      status.className = 'restore-step-status';
      status.textContent = BackupsPage.statusLabel(step.status);
      li.appendChild(status);
      const detail = extractStepDetail(step);
      if (detail) {
        const detailEl = document.createElement('div');
        detailEl.className = 'restore-step-detail muted';
        detailEl.textContent = detail;
        li.appendChild(detailEl);
      }
      list.appendChild(li);
    });
  }

  function extractStepDetail(step) {
    if (!step || typeof step !== 'object') return '';
    const d = step.details || {};
    if (typeof d.error === 'string' && d.error.trim()) return d.error.trim();
    return '';
  }

  function toggleLoading(show) {
    const loading = document.getElementById('backups-restore-loading');
    if (loading) loading.hidden = !show;
  }

  function openImportModal() {
    const modal = document.getElementById('backups-import-modal');
    const fileInput = document.getElementById('backups-import-file');
    if (fileInput) fileInput.value = '';
    if (modal) modal.hidden = false;
  }

  function closeImportModal() {
    const modal = document.getElementById('backups-import-modal');
    if (modal) modal.hidden = true;
  }

  async function submitImport() {
    const fileInput = document.getElementById('backups-import-file');
    const file = fileInput?.files?.[0];
    if (!file) {
      BackupsPage.setAlert('error', 'backups.error.invalidFormat', BackupsPage.t('backups.error.invalidFormat'));
      return;
    }
    const formData = new FormData();
    formData.append('file', file, file.name || 'uploaded.bscc');
    toggleImportBusy(true);
    BackupsPage.setAlert('', '', '');
    try {
      await Api.upload('/api/backups/import', formData);
      closeImportModal();
      BackupsPage.setAlert('success', 'backups.import.success', '');
      await load(true);
      if (typeof BackupsHistory !== 'undefined') BackupsHistory.load(true);
      if (typeof BackupsOverview !== 'undefined') BackupsOverview.load();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('backups.import.failed'));
    } finally {
      toggleImportBusy(false);
    }
  }

  function toggleImportBusy(disabled) {
    const ids = ['backups-import-submit', 'backups-import-cancel', 'backups-import-cancel-top', 'backups-import-file'];
    ids.forEach((id) => {
      const el = document.getElementById(id);
      if (el) el.disabled = disabled;
    });
  }

  return { init, load };
})();
