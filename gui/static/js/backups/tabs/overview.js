const BackupsOverview = (() => {
  function init() {
    bindActions();
    bindScopeControls();
  }

  function bindActions() {
    const createBtn = document.getElementById('backups-create-now');
    const refreshBtn = document.getElementById('backups-overview-refresh');
    const emptyCreate = document.getElementById('backups-overview-empty-create');
    const runIntegrityBtn = document.getElementById('backups-overview-run-integrity');
    if (createBtn) createBtn.addEventListener('click', createNow);
    if (refreshBtn) refreshBtn.addEventListener('click', () => load(true));
    if (emptyCreate) emptyCreate.addEventListener('click', createNow);
    if (runIntegrityBtn) runIntegrityBtn.addEventListener('click', runIntegrityTest);
  }

  async function createNow() {
    toggleLoading(true);
    disableCreate(true);
    BackupsPage.setAlert('', '', '');
    try {
      const res = await BackupsPage.createBackup(buildCreatePayload());
      const key = res?.run?.status === 'success' ? 'backups.create.success' : 'backups.create.started';
      BackupsPage.setAlert('success', key, '');
      await load(false);
      if (typeof BackupsHistory !== 'undefined') BackupsHistory.load();
      if (typeof BackupsRestore !== 'undefined') BackupsRestore.load();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
    } finally {
      disableCreate(false);
      toggleLoading(false);
    }
  }

  function bindScopeControls() {
    document.querySelectorAll('#backups-create-scope-list input[type="checkbox"]').forEach((el) => {
      el.addEventListener('change', () => {
        syncScopeSelection(el.dataset.scope || '');
      });
    });
    syncScopeSelection('all');
  }

  function syncScopeSelection(changedScope) {
    const checkboxes = Array.from(document.querySelectorAll('#backups-create-scope-list input[type="checkbox"]'));
    const allBox = checkboxes.find((el) => (el.dataset.scope || '') === 'all');
    const moduleBoxes = checkboxes.filter((el) => (el.dataset.scope || '') !== 'all');
    if (changedScope === 'all' && allBox && allBox.checked) {
      moduleBoxes.forEach((el) => { el.checked = false; });
    }
    if (changedScope !== 'all') {
      const hasModules = moduleBoxes.some((el) => el.checked);
      if (allBox) allBox.checked = !hasModules;
    }
    const selected = selectedScope();
    const preview = document.getElementById('backups-create-scope-preview');
    if (preview) preview.textContent = selected.length === 1 && selected[0] === 'ALL' ? 'ALL' : selected.join(', ');
  }

  function selectedScope() {
    const all = document.querySelector('#backups-create-scope-list input[data-scope="all"]');
    if (all && all.checked) return ['ALL'];
    const selected = Array.from(document.querySelectorAll('#backups-create-scope-list input[type="checkbox"]'))
      .filter((el) => (el.dataset.scope || '') !== 'all' && el.checked)
      .map((el) => (el.dataset.scope || '').trim())
      .filter(Boolean);
    if (!selected.length) return ['ALL'];
    return selected;
  }

  function buildCreatePayload() {
    return {
      label: (document.getElementById('backups-create-label')?.value || '').trim(),
      scope: selectedScope(),
      include_files: !!document.getElementById('backups-create-include-files')?.checked,
    };
  }

  async function load(showSpinner = false) {
    if (showSpinner) toggleLoading(true);
    BackupsPage.setAlert('', '', '');
    try {
      const [items, integrityRes] = await Promise.all([
        BackupsPage.listBackups(),
        Api.get('/api/backups/integrity').catch(() => ({ item: null })),
      ]);
      render(items, integrityRes?.item || null);
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      render([], null);
    } finally {
      toggleLoading(false);
    }
  }

  function render(items, integrity) {
    const lastRun = (items || [])[0] || null;
    const lastSuccessful = (items || []).find((item) => item.status === 'success') || null;

    setText('backups-overview-last-success-date', BackupsPage.formatDateTime(lastSuccessful?.created_at));
    setText('backups-overview-last-success-size', BackupsPage.formatBytes(lastSuccessful?.size_bytes));
    setText('backups-overview-last-success-file', lastSuccessful?.filename || '-');

    setText('backups-overview-last-run-status', lastRun ? BackupsPage.statusLabel(lastRun.status) : '-');
    setText('backups-overview-last-run-updated', BackupsPage.formatDateTime(lastRun?.updated_at));
    renderIntegrity(integrity);

    const empty = document.getElementById('backups-overview-empty');
    if (empty) empty.hidden = (items || []).length > 0;
  }

  function disableCreate(disabled) {
    const createBtn = document.getElementById('backups-create-now');
    const emptyCreate = document.getElementById('backups-overview-empty-create');
    if (createBtn) createBtn.disabled = disabled;
    if (emptyCreate) emptyCreate.disabled = disabled;
  }

  function toggleLoading(show) {
    const loading = document.getElementById('backups-overview-loading');
    if (loading) loading.hidden = !show;
  }

  function renderIntegrity(item) {
    const statusMap = {
      ok: 'backups.integrity.state.ok',
      warning: 'backups.integrity.state.warning',
      failed: 'backups.integrity.state.failed',
    };
    const statusLabel = item?.status ? BackupsPage.t(statusMap[item.status] || '') || item.status : '-';
    const lastLabel = item?.last_restore_test_at ? `${BackupsPage.formatDateTime(item.last_restore_test_at)} (${BackupsPage.statusLabel(item.last_restore_test_status)})` : '-';
    setText('backups-overview-integrity-status', statusLabel);
    setText('backups-overview-integrity-last-test', lastLabel);
    setText('backups-overview-integrity-next-test', BackupsPage.formatDateTime(item?.next_scheduled_test_at));
  }

  async function runIntegrityTest() {
    BackupsPage.setAlert('', '', '');
    try {
      await Api.post('/api/backups/integrity/run', {});
      BackupsPage.setAlert('success', 'backups.integrity.queued', '');
      await load(false);
      if (typeof BackupsRestore !== 'undefined') BackupsRestore.load();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
    }
  }

  function setText(id, value) {
    const el = document.getElementById(id);
    if (el) el.textContent = value ?? '-';
  }

  return { init, load };
})();
