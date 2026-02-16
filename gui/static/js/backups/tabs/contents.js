const BackupsContents = (() => {
  const state = {
    items: [],
    selectedId: 0,
  };

  function init() {
    const select = document.getElementById('backups-contents-artifact');
    const refresh = document.getElementById('backups-contents-refresh');
    if (select) {
      select.addEventListener('change', () => {
        state.selectedId = parseInt(select.value, 10) || 0;
        loadSelected();
      });
    }
    if (refresh) refresh.addEventListener('click', () => load(true));
  }

  async function load(showSpinner = false) {
    if (showSpinner) toggleLoading(true);
    try {
      state.items = await BackupsPage.listBackups();
      renderSelect();
      await loadSelected();
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      state.items = [];
      renderSelect();
      renderEmpty(true);
    } finally {
      toggleLoading(false);
    }
  }

  function renderSelect() {
    const select = document.getElementById('backups-contents-artifact');
    if (!select) return;
    const previous = state.selectedId || parseInt(select.value, 10) || 0;
    select.innerHTML = '';
    const placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.textContent = BackupsPage.t('backups.contents.placeholder');
    select.appendChild(placeholder);
    (state.items || []).forEach((item) => {
      const option = document.createElement('option');
      option.value = String(item.id);
      option.textContent = item.filename || `#${item.id}`;
      select.appendChild(option);
    });
    if (previous && state.items.some((item) => item.id === previous)) {
      state.selectedId = previous;
      select.value = String(previous);
    } else {
      state.selectedId = parseInt(select.value, 10) || 0;
    }
  }

  async function loadSelected() {
    const id = state.selectedId;
    if (!id) {
      renderEmpty(true);
      return;
    }
    toggleLoading(true);
    try {
      const res = await BackupsPage.apiGet(`/api/backups/${id}?resource=contents`);
      const item = res?.item || null;
      renderItem(item);
    } catch (err) {
      const e = BackupsPage.parseError(err);
      BackupsPage.setAlert('error', e.i18nKey, BackupsPage.t('common.serverError'));
      renderEmpty(true);
    } finally {
      toggleLoading(false);
    }
  }

  function renderItem(item) {
    if (!item) {
      renderEmpty(true);
      return;
    }
    renderEmpty(false);
    const meta = parseMeta(item.meta_json);
    setText('backups-contents-filename', item.filename || '-');
    setText('backups-contents-status', BackupsPage.statusLabel(item.status));
    setText('backups-contents-created', BackupsPage.formatDateTime(item.created_at));
    setText('backups-contents-size', BackupsPage.formatBytes(item.size_bytes));
    setText('backups-contents-checksum', item.checksum || '-');
    setText('backups-contents-scope', renderScope(meta.backup_scope));
    setText('backups-contents-monitor-count', renderMonitorCount(meta));
    setText('backups-contents-include-files', meta.includes_files ? BackupsPage.t('common.yes') : BackupsPage.t('common.no'));
    setText('backups-contents-label', meta.backup_label || '-');
    setText('backups-contents-app-version', meta.app_version || '-');
    setText('backups-contents-db-engine', meta.db_engine || '-');
    setText('backups-contents-goose-version', meta.goose_db_version || '-');
    setText('backups-contents-format-version', meta.format_version || '-');
    renderChecksums(meta.checksums);
    renderEntityCounts(meta.entity_counts);
  }

  function renderChecksums(checksums) {
    const tbody = document.querySelector('#backups-contents-checksums-table tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    const entries = buildChecksumEntries(checksums);
    if (!entries.length) {
      const row = document.createElement('tr');
      const td = document.createElement('td');
      td.colSpan = 3;
      td.className = 'muted';
      td.textContent = BackupsPage.t('common.empty');
      row.appendChild(td);
      tbody.appendChild(row);
      return;
    }
    entries.sort((a, b) => a.name.localeCompare(b.name)).forEach((entry) => {
      const row = document.createElement('tr');
      row.appendChild(cell(entry.name));
      row.appendChild(cell(entry.sha256 || '-'));
      row.appendChild(cell(BackupsPage.formatBytes(entry.size)));
      tbody.appendChild(row);
    });
  }

  function buildChecksumEntries(checksums) {
    if (!checksums || typeof checksums !== 'object') return [];
    if (typeof checksums.manifest_sha256 === 'string' || typeof checksums.dump_sha256 === 'string') {
      return [
        { name: 'manifest.json', sha256: checksums.manifest_sha256 || '-', size: null },
        { name: 'db.dump', sha256: checksums.dump_sha256 || '-', size: null },
      ];
    }
    return Object.entries(checksums).map(([name, info]) => ({
      name,
      sha256: info && info.sha256 ? info.sha256 : '',
      size: info && info.size ? info.size : null,
    }));
  }

  function renderScope(scope) {
    if (!Array.isArray(scope) || !scope.length) return 'ALL';
    return scope.join(', ');
  }

  function renderMonitorCount(meta) {
    if (!meta || typeof meta !== 'object') return '-';
    const counts = meta.entity_counts;
    if (!counts || typeof counts !== 'object') return '-';
    const raw = counts['monitoring.monitors'];
    if (raw === null || raw === undefined || Number.isNaN(Number(raw))) return '-';
    return String(Number(raw));
  }

  function renderEntityCounts(entityCounts) {
    const tbody = document.querySelector('#backups-contents-entities-table tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    if (!entityCounts || typeof entityCounts !== 'object') {
      const row = document.createElement('tr');
      const td1 = document.createElement('td');
      td1.colSpan = 2;
      td1.className = 'muted';
      td1.textContent = BackupsPage.t('common.empty');
      row.appendChild(td1);
      tbody.appendChild(row);
      return;
    }
    const entries = Object.entries(entityCounts);
    if (!entries.length) {
      const row = document.createElement('tr');
      const td1 = document.createElement('td');
      td1.colSpan = 2;
      td1.className = 'muted';
      td1.textContent = BackupsPage.t('common.empty');
      row.appendChild(td1);
      tbody.appendChild(row);
      return;
    }
    entries
      .sort((a, b) => a[0].localeCompare(b[0]))
      .forEach(([key, value]) => {
        const row = document.createElement('tr');
        row.appendChild(cell(entityLabel(key)));
        row.appendChild(cell(Number.isFinite(Number(value)) ? String(Number(value)) : '-'));
        tbody.appendChild(row);
      });
  }

  function entityLabel(key) {
    const i18nKey = `backups.contents.entity.${key}`;
    const translated = BackupsPage.t(i18nKey);
    if (translated && translated !== i18nKey) return translated;
    return key;
  }

  function parseMeta(raw) {
    if (!raw) return {};
    if (typeof raw === 'object') return raw;
    try {
      return JSON.parse(raw);
    } catch (_) {
      return {};
    }
  }

  function renderEmpty(show) {
    const empty = document.getElementById('backups-contents-empty');
    const body = document.getElementById('backups-contents-body');
    if (empty) empty.hidden = !show;
    if (body) body.hidden = show;
  }

  function toggleLoading(show) {
    const loading = document.getElementById('backups-contents-loading');
    if (loading) loading.hidden = !show;
  }

  function setText(id, value) {
    const el = document.getElementById(id);
    if (el) el.textContent = value ?? '-';
  }

  function cell(value) {
    const td = document.createElement('td');
    td.textContent = value ?? '-';
    return td;
  }

  return { init, load };
})();
