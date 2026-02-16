const BackupsHistory = (() => {
  const state = {
    items: [],
    deleteId: 0,
  };

  function init() {
    bindActions();
  }

  function bindActions() {
    const status = document.getElementById('backups-history-status');
    const search = document.getElementById('backups-history-search');
    const refresh = document.getElementById('backups-history-refresh');
    const emptyCreate = document.getElementById('backups-history-empty-create');
    if (status) status.addEventListener('change', render);
    if (search) search.addEventListener('input', render);
    if (refresh) refresh.addEventListener('click', () => load(true));
    if (emptyCreate) {
      emptyCreate.addEventListener('click', () => {
        const createBtn = document.getElementById('backups-create-now');
        if (createBtn) createBtn.click();
      });
    }

    const confirmBtn = document.getElementById('backups-delete-confirm');
    const cancelBtn = document.getElementById('backups-delete-cancel');
    const cancelTop = document.getElementById('backups-delete-cancel-top');
    if (confirmBtn) confirmBtn.addEventListener('click', confirmDelete);
    if (cancelBtn) cancelBtn.addEventListener('click', closeDeleteModal);
    if (cancelTop) cancelTop.addEventListener('click', closeDeleteModal);
  }

  async function load(showSpinner = false) {
    if (showSpinner) toggleLoading(true);
    try {
      state.items = await BackupsPage.listBackups();
      render();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      state.items = [];
      render();
    } finally {
      toggleLoading(false);
    }
  }

  function filteredItems() {
    const selectedStatus = (document.getElementById('backups-history-status')?.value || '').trim();
    const searchRaw = (document.getElementById('backups-history-search')?.value || '').trim().toLowerCase();
    return (state.items || []).filter((item) => {
      if (selectedStatus && item.status !== selectedStatus) return false;
      if (searchRaw) {
        const filename = (item.filename || '').toLowerCase();
        if (!filename.includes(searchRaw)) return false;
      }
      return true;
    });
  }

  function render() {
    const items = filteredItems();
    const tbody = document.querySelector('#backups-history-table tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    if (!items.length) {
      const row = document.createElement('tr');
      const cell = document.createElement('td');
      cell.colSpan = 5;
      cell.className = 'muted';
      cell.textContent = BackupsPage.t('backups.empty.noBackups');
      row.appendChild(cell);
      tbody.appendChild(row);
    } else {
      items.forEach((item) => tbody.appendChild(renderRow(item)));
    }
    const empty = document.getElementById('backups-history-empty');
    if (empty) empty.hidden = state.items.length > 0;
  }

  function renderRow(item) {
    const row = document.createElement('tr');
    row.appendChild(cell(BackupsPage.formatDateTime(item.created_at)));
    row.appendChild(cell(BackupsPage.statusLabel(item.status)));
    row.appendChild(cell(BackupsPage.formatBytes(item.size_bytes)));
    row.appendChild(cell(item.filename || '-'));

    const actions = document.createElement('td');
    const downloadBtn = document.createElement('button');
    downloadBtn.className = 'btn ghost';
    downloadBtn.textContent = BackupsPage.t('backups.actions.download');
    downloadBtn.addEventListener('click', () => download(item.id));
    actions.appendChild(downloadBtn);

    const deleteBtn = document.createElement('button');
    deleteBtn.className = 'btn danger backups-action-spaced';
    deleteBtn.textContent = BackupsPage.t('backups.actions.delete');
    deleteBtn.addEventListener('click', () => openDeleteModal(item.id, item.filename || `#${item.id}`));
    actions.appendChild(deleteBtn);

    row.appendChild(actions);
    return row;
  }

  function cell(value) {
    const td = document.createElement('td');
    td.textContent = value ?? '-';
    return td;
  }

  function download(id) {
    window.location.href = `/api/backups/${id}/download`;
  }

  function openDeleteModal(id, label) {
    state.deleteId = id;
    const modal = document.getElementById('backups-delete-modal');
    const msg = document.getElementById('backups-delete-message');
    if (msg) msg.textContent = `${BackupsPage.t('backups.delete.confirmMessage')} ${label}`;
    if (modal) modal.hidden = false;
  }

  function closeDeleteModal() {
    state.deleteId = 0;
    const modal = document.getElementById('backups-delete-modal');
    if (modal) modal.hidden = true;
  }

  async function confirmDelete() {
    if (!state.deleteId) return;
    try {
      await BackupsPage.apiDelete(`/api/backups/${state.deleteId}`);
      BackupsPage.setAlert('success', 'backups.delete.success', '');
      closeDeleteModal();
      await load(true);
      if (typeof BackupsOverview !== 'undefined') BackupsOverview.load();
      if (typeof BackupsRestore !== 'undefined') BackupsRestore.load();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      closeDeleteModal();
    }
  }

  function toggleLoading(show) {
    const loading = document.getElementById('backups-history-loading');
    if (loading) loading.hidden = !show;
  }

  return { init, load };
})();
