(() => {
  const state = IncidentsPage.state;
  const { t, showError, StageBlocks, formatDate, syncIncident } = IncidentsPage;
  const modalState = {
    stageType: 'investigation',
    usePreset: true,
    selectedBlocks: [],
    requiredBlocks: []
  };

  function stageTypeLabel(stage) {
    if (stage?.is_default) return t('incidents.stage.overview');
    const code = stage?.stageType || 'custom';
    const label = t(`incidents.stage.type.${code}`);
    if (label && !label.startsWith('incidents.')) return label;
    return code || t('incidents.stage.type.custom');
  }

  function stagePreset(type) {
    return StageBlocks?.STAGE_PRESETS?.[type] || null;
  }

  function ensureRequiredBlocks() {
    const present = new Set(modalState.selectedBlocks);
    modalState.requiredBlocks.forEach(req => {
      if (!present.has(req)) {
        modalState.selectedBlocks.unshift(req);
        present.add(req);
      }
    });
    modalState.selectedBlocks = modalState.selectedBlocks.filter((type, idx, arr) => arr.indexOf(type) === idx);
  }

  function applyPresetSelection(type) {
    modalState.stageType = type;
    const preset = stagePreset(type);
    modalState.requiredBlocks = preset?.required || [];
    const defaults = preset?.blocks || ['note', 'checklist'];
    modalState.selectedBlocks = defaults.slice();
    modalState.usePreset = true;
    if (type === 'closure') {
      modalState.requiredBlocks = ['decisions'];
      modalState.selectedBlocks = ['decisions'];
    }
    renderStageBlockSelector();
    syncPresetToggle();
    updateTypeDescription();
  }

  function updateTypeDescription() {
    const descEl = document.getElementById('incident-stage-type-description');
    if (!descEl) return;
    descEl.textContent = '';
    descEl.hidden = true;
  }

  function syncPresetToggle() {
    const toggle = document.getElementById('incident-stage-use-preset');
    if (toggle) {
      toggle.checked = !!modalState.usePreset;
      toggle.disabled = modalState.stageType === 'closure';
    }
  }

  function renderStageBlockSelector() {
    const selectedWrap = document.getElementById('incident-stage-blocks');
    const availableWrap = document.getElementById('incident-stage-blocks-available');
    if (!selectedWrap) return;
    ensureRequiredBlocks();
    const isClosure = modalState.stageType === 'closure';
    const allBlocks = (StageBlocks?.BLOCK_ORDER && StageBlocks.BLOCK_ORDER.length)
      ? StageBlocks.BLOCK_ORDER
      : ['note', 'checklist', 'actions', 'decisions', 'timeline', 'artifacts', 'links', 'table'];
    const available = allBlocks.filter(type => !modalState.selectedBlocks.includes(type));
    selectedWrap.innerHTML = '';
    modalState.selectedBlocks.forEach((type, idx) => {
      const row = document.createElement('div');
      row.className = 'stage-block-option';
      row.dataset.type = type;
      const info = document.createElement('div');
      info.className = 'stage-block-option-info';
      const title = document.createElement('div');
      title.className = 'stage-block-option-title';
      title.textContent = StageBlocks?.blockTitle(type) || type;
      info.appendChild(title);
      const hintText = StageBlocks?.blockHint(modalState.stageType, type);
      if (hintText) {
        const hint = document.createElement('div');
        hint.className = 'stage-block-option-hint';
        hint.textContent = hintText;
        info.appendChild(hint);
      }
      if (modalState.requiredBlocks.includes(type)) {
        const badge = document.createElement('span');
        badge.className = 'pill pill-badge stage-block-required';
        badge.textContent = t('incidents.stage.blocks.required');
        info.appendChild(badge);
      }
      row.appendChild(info);
      const controls = document.createElement('div');
      controls.className = 'stage-block-option-actions';
      const up = document.createElement('button');
      up.type = 'button';
      up.className = 'btn ghost icon-btn reorder-btn';
      up.textContent = '↑';
      up.disabled = isClosure || idx === 0;
      up.addEventListener('click', () => moveBlock(type, -1));
      const down = document.createElement('button');
      down.type = 'button';
      down.className = 'btn ghost icon-btn reorder-btn';
      down.textContent = '↓';
      down.disabled = isClosure || idx === modalState.selectedBlocks.length - 1;
      down.addEventListener('click', () => moveBlock(type, 1));
      const toggle = document.createElement('button');
      toggle.type = 'button';
      toggle.className = 'btn ghost icon-btn';
      toggle.textContent = '×';
      toggle.disabled = isClosure || modalState.requiredBlocks.includes(type);
      const removeLabel = t('common.delete');
      toggle.title = removeLabel && !removeLabel.startsWith('common.') ? removeLabel : 'Remove';
      toggle.addEventListener('click', () => {
        removeBlock(type);
      });
      controls.appendChild(up);
      controls.appendChild(down);
      controls.appendChild(toggle);
      row.appendChild(controls);
      selectedWrap.appendChild(row);
    });
    if (availableWrap) {
      availableWrap.innerHTML = '';
      const label = document.createElement('div');
      label.className = 'form-hint';
      label.textContent = t('incidents.stage.blocks.available');
      availableWrap.appendChild(label);
      const controls = document.createElement('div');
      controls.className = 'stage-block-available-controls';
      const select = document.createElement('select');
      select.className = 'select';
      select.id = 'incident-stage-blocks-select';
      select.disabled = isClosure;
      const allForSelect = allBlocks.length ? allBlocks : ['note', 'checklist', 'actions', 'decisions', 'timeline', 'artifacts', 'links', 'table'];
      if (!allForSelect.length) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = t('incidents.stage.blocks.noneAvailable') || '-';
        select.appendChild(opt);
      } else {
        allForSelect.forEach(type => {
          const opt = document.createElement('option');
          opt.value = type;
          opt.textContent = StageBlocks?.blockTitle(type) || type;
          if (modalState.selectedBlocks.includes(type)) {
            opt.disabled = true;
          }
          select.appendChild(opt);
        });
      }
      controls.appendChild(select);
      const addBtn = document.createElement('button');
      addBtn.type = 'button';
      addBtn.className = 'btn ghost';
      addBtn.id = 'incident-stage-blocks-add';
      addBtn.textContent = t('incidents.stage.blocks.addOptional') || 'Add block';
      addBtn.disabled = isClosure || available.length === 0;
      addBtn.addEventListener('click', () => {
        addBlock(select.value);
      });
      controls.appendChild(addBtn);
      availableWrap.appendChild(controls);
    }
  }

  function moveBlock(type, delta) {
    const idx = modalState.selectedBlocks.indexOf(type);
    if (idx === -1) return;
    const nextIdx = idx + delta;
    if (nextIdx < 0 || nextIdx >= modalState.selectedBlocks.length) return;
    modalState.usePreset = false;
    const reordered = modalState.selectedBlocks.slice();
    const [item] = reordered.splice(idx, 1);
    reordered.splice(nextIdx, 0, item);
    modalState.selectedBlocks = reordered;
    syncPresetToggle();
    renderStageBlockSelector();
  }

  function removeBlock(type) {
    if (modalState.requiredBlocks.includes(type)) return;
    modalState.usePreset = false;
    modalState.selectedBlocks = modalState.selectedBlocks.filter(b => b !== type);
    syncPresetToggle();
    renderStageBlockSelector();
  }

  function addBlock(type) {
    if (!type || modalState.selectedBlocks.includes(type)) return;
    modalState.usePreset = false;
    modalState.selectedBlocks.push(type);
    syncPresetToggle();
    renderStageBlockSelector();
  }

  function renderIncidentStages(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    detail.stages.forEach(stage => {
      const status = (stage.status || 'open').toLowerCase();
      stage.status = status;
      stage.readOnly = detail.readOnly || status === 'done';
    });
    const tabs = panel.querySelector('.incident-stage-tabs');
    const content = panel.querySelector('.incident-stage-content');
    if (!tabs) return;
    tabs.innerHTML = '';
    detail.stages.forEach(stage => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'tab-btn';
      if (detail.activeStageId === stage.id) btn.classList.add('active');
      btn.dataset.stage = stage.id;
      const title = document.createElement('span');
      title.className = 'tab-title';
      title.textContent = stage.is_default ? t('incidents.stage.overview') : stage.title;
      btn.appendChild(title);
      if (stage.closable && !stage.readOnly && !detail.readOnly) {
        const close = document.createElement('span');
        close.className = 'tab-close';
        close.textContent = 'x';
        close.setAttribute('role', 'button');
        close.setAttribute('aria-label', t('common.close') || 'Close');
        close.addEventListener('click', (e) => {
          e.stopPropagation();
          requestCloseStage(incidentId, stage.id);
        });
        btn.appendChild(close);
      }
      btn.addEventListener('click', () => {
        detail.activeStageId = stage.id;
        detail.activeInnerTab = 'stages';
        if (IncidentsPage.renderIncidentInnerTabs) {
          IncidentsPage.renderIncidentInnerTabs(incidentId);
        }
        if (IncidentsPage.renderIncidentInnerContent) {
          IncidentsPage.renderIncidentInnerContent(incidentId);
        } else {
          renderIncidentStages(incidentId);
        }
      });
      tabs.appendChild(btn);
    });
    if (!content) return;
    const active = detail.stages.find(s => s.id === detail.activeStageId) || detail.stages[0];
    if (!active) return;
    const passport = panel.querySelector('.incident-passport');
    if (passport) passport.hidden = !(detail.activeInnerTab === 'stages' && active.is_default);
    content.innerHTML = '';
    const panelEl = document.createElement('div');
    panelEl.className = 'stage-panel';
    if (active.is_default) {
      panelEl.classList.add('stage-overview-panel');
    } else {
      const header = document.createElement('div');
      header.className = 'stage-panel-header';
      const headerInfo = document.createElement('div');
      headerInfo.className = 'stage-header-info';
      const title = document.createElement('h4');
      title.textContent = active.title;
      headerInfo.appendChild(title);
      const helperText = StageBlocks?.stageHelper(active.stageType);
      if (helperText) {
        const helper = document.createElement('div');
        helper.className = 'stage-helper';
        helper.textContent = helperText;
        headerInfo.appendChild(helper);
      }
      header.appendChild(headerInfo);
      const metaWrap = document.createElement('div');
      metaWrap.className = 'stage-meta';
      const meta = document.createElement('span');
      meta.className = 'stage-type-pill pill';
      meta.textContent = stageTypeLabel(active);
      metaWrap.appendChild(meta);
      const statusPill = document.createElement('span');
      statusPill.className = `stage-status-pill pill status-${active.status || 'open'}`;
      statusPill.textContent = t(`incidents.stage.status.${active.status || 'open'}`);
      metaWrap.appendChild(statusPill);
      header.appendChild(metaWrap);
      const headerActions = document.createElement('div');
      headerActions.className = 'stage-header-actions';
      const completeBtn = document.createElement('button');
      completeBtn.type = 'button';
      completeBtn.className = 'btn ghost stage-complete';
      completeBtn.textContent = t('incidents.stage.complete');
      completeBtn.disabled = active.readOnly;
      completeBtn.addEventListener('click', () => completeStage(incidentId, active.id));
      headerActions.appendChild(completeBtn);
      header.appendChild(headerActions);
      panelEl.appendChild(header);
    }
    const blocks = document.createElement('div');
    blocks.className = 'stage-blocks';
    if (active.readOnly) {
      panelEl.classList.add('stage-readonly');
    }
    if (active.is_default) {
      renderStageOverview(incidentId, detail, blocks);
    } else if (StageBlocks && StageBlocks.renderBlocks) {
      active.stageType = active.stageType || 'custom';
      StageBlocks.ensureStageDefaults(active);
      StageBlocks.renderBlocks(active, blocks, {
        incidentId,
        onChange: () => {
          if (active.readOnly) return;
          active.currentSerialized = StageBlocks.serializeContent(active);
          if (IncidentsPage.updateIncidentSaveState) {
            IncidentsPage.updateIncidentSaveState(incidentId);
          }
        }
      });
      if (active.readOnly) {
        disableStageEditing(blocks);
      }
    }
    panelEl.appendChild(blocks);
    if (!active.is_default && active.status === 'done') {
      const hint = renderStageCompletionHint(incidentId, detail, active);
      if (hint) panelEl.appendChild(hint);
    }
    content.appendChild(panelEl);
  }

  function openStageModal(incidentId) {
    state.pendingStageIncidentId = incidentId;
    const modal = document.getElementById('incident-stage-modal');
    const input = document.getElementById('incident-stage-name');
    const typeSelect = document.getElementById('incident-stage-type');
    if (input) input.value = '';
    if (typeSelect) typeSelect.value = 'investigation';
    modalState.stageType = 'investigation';
    modalState.usePreset = true;
    applyPresetSelection('investigation');
    if (modal) modal.hidden = false;
    if (input) input.focus();
  }

  function bindStageModal() {
    const confirm = document.getElementById('incident-stage-confirm');
    const cancel = document.getElementById('incident-stage-cancel');
    const close = document.getElementById('close-incident-stage-modal');
    const typeSelect = document.getElementById('incident-stage-type');
    const presetToggle = document.getElementById('incident-stage-use-preset');
    if (confirm) confirm.addEventListener('click', () => submitStageModal());
    if (cancel) cancel.addEventListener('click', () => closeStageModal());
    if (close) close.addEventListener('click', () => closeStageModal());
    if (typeSelect) {
      typeSelect.addEventListener('change', () => {
        modalState.stageType = typeSelect.value || 'custom';
        if (modalState.stageType === 'closure') {
          modalState.usePreset = true;
        }
        if (modalState.usePreset) {
          applyPresetSelection(modalState.stageType);
        } else {
          modalState.requiredBlocks = stagePreset(modalState.stageType)?.required || [];
          ensureRequiredBlocks();
          updateTypeDescription();
          renderStageBlockSelector();
        }
        syncPresetToggle();
      });
    }
    if (presetToggle) {
      presetToggle.addEventListener('change', () => {
        modalState.usePreset = presetToggle.checked;
        if (modalState.usePreset) {
          applyPresetSelection(modalState.stageType);
        } else {
          renderStageBlockSelector();
        }
      });
    }
  }

  function collectStageBlocks() {
    return modalState.selectedBlocks.slice();
  }

  async function submitStageModal() {
    const modal = document.getElementById('incident-stage-modal');
    const input = document.getElementById('incident-stage-name');
    const alertBox = document.getElementById('incident-stage-alert');
    const incidentId = state.pendingStageIncidentId;
    if (!incidentId || !input) return;
    if (alertBox) {
      input.addEventListener('input', () => {
        if (input.value.trim()) alertBox.hidden = true;
      }, { once: true });
    }
    const name = input.value.trim();
    if (!name) {
      if (alertBox) {
        const msg = t('incidents.stageTitleRequired');
        alertBox.textContent = msg && !msg.startsWith('incidents.') ? msg : 'Title is required';
        alertBox.hidden = false;
      }
      input.focus();
      return;
    }
    if (alertBox) alertBox.hidden = true;
    const detail = state.incidentDetails.get(incidentId);
    if (!detail) return;
    const stageType = modalState.stageType || 'custom';
    const selectedBlocks = collectStageBlocks();
    const blocks = StageBlocks ? StageBlocks.createBlocksFromSelection(selectedBlocks, stageType) : [];
    const serialized = StageBlocks ? StageBlocks.serializeContent({ stageType, blocks }) : '';
    try {
      const res = await Api.post(`/api/incidents/${incidentId}/stages`, { title: name });
      const stage = res.stage;
      const entry = res.entry || {};
      const newStage = {
        id: stage.id,
        title: stage.title,
        is_default: !!stage.is_default,
        closable: !stage.is_default,
        position: stage.position,
        version: stage.version,
        entryVersion: entry.version || 1,
        status: (stage.status || 'open'),
        closed_at: stage.closed_at,
        closed_by: stage.closed_by,
        readOnly: detail.readOnly || (stage.status || 'open') === 'done',
        stageType,
        blocks,
        initialSerialized: serialized,
        currentSerialized: serialized
      };
      detail.stages.push(newStage);
      detail.activeStageId = stage.id;
      renderIncidentStages(incidentId);
      if (StageBlocks && serialized) {
        try {
          const saved = await Api.put(`/api/incidents/${incidentId}/stages/${stage.id}/content`, {
            content: serialized,
            change_reason: '',
            version: entry.version || 1,
          });
          newStage.entryVersion = saved.version || newStage.entryVersion;
          newStage.initialSerialized = serialized;
          newStage.currentSerialized = serialized;
        } catch (err) {
          newStage.initialSerialized = '';
          newStage.currentSerialized = serialized;
          showError(err, 'common.error');
        }
      }
      if (IncidentsPage.updateIncidentSaveState) {
        IncidentsPage.updateIncidentSaveState(incidentId);
      }
      if (modal) modal.hidden = true;
      if (alertBox) alertBox.hidden = true;
    } catch (err) {
      showError(err, 'incidents.stageTitleRequired');
    }
  }

  function closeStageModal() {
    const modal = document.getElementById('incident-stage-modal');
    if (modal) modal.hidden = true;
  }

  async function requestCloseStage(incidentId, stageId) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail) return;
    const stage = detail.stages.find(s => s.id === stageId);
    if (!stage || !stage.closable || stage.readOnly || detail.readOnly) return;
    const confirmMsg = isStageDirty(stage) ? t('incidents.stageCloseUnsaved') : t('incidents.stageDeleteConfirm');
    const ok = await IncidentsPage.confirmAction({
      title: t('common.confirm'),
      message: confirmMsg,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
    });
    if (!ok) return;
    try {
      await Api.del(`/api/incidents/${incidentId}/stages/${stageId}`);
      detail.stages = detail.stages.filter(s => s.id !== stageId);
      if (detail.activeStageId === stageId) {
        detail.activeStageId = detail.stages[0]?.id || null;
      }
      renderIncidentStages(incidentId);
    } catch (err) {
      showError(err, 'incidents.cannotDeleteOverview');
    }
  }

  async function saveStageContent(incidentId, stage, opts = {}) {
    if (!stage || stage.saving || stage.is_default || stage.readOnly) return false;
    if (!isStageDirty(stage)) return true;
    stage.saving = true;
    try {
      const payload = StageBlocks ? StageBlocks.serializeContent(stage) : (stage.currentSerialized || '');
      const res = await Api.put(`/api/incidents/${incidentId}/stages/${stage.id}/content`, {
        content: payload,
        change_reason: '',
        version: stage.entryVersion || 1,
      });
      stage.entryVersion = res.version || stage.entryVersion + 1;
      stage.initialSerialized = payload;
      stage.currentSerialized = payload;
      if (IncidentsPage.updateIncidentSaveState) {
        IncidentsPage.updateIncidentSaveState(incidentId);
      }
      return true;
    } catch (err) {
      if (!opts.silent) {
        showError(err, 'incidents.conflictVersion');
      }
      return false;
    } finally {
      stage.saving = false;
    }
  }

  function isStageDirty(stage) {
    if (!stage || stage.is_default || stage.readOnly) return false;
    if (!StageBlocks) return false;
    const current = stage.currentSerialized || StageBlocks.serializeContent(stage);
    const initial = stage.initialSerialized || '';
    return current !== initial;
  }

  async function saveDirtyStages(incidentId, opts = {}) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail || detail.readOnly) return false;
    const dirtyStages = detail.stages.filter(isStageDirty);
    if (!dirtyStages.length) return false;
    let allSaved = true;
    for (const stage of dirtyStages) {
      const ok = await saveStageContent(incidentId, stage, opts);
      if (!ok) allSaved = false;
    }
    return allSaved;
  }

  function disableStageEditing(container) {
    if (!container) return;
    container.classList.add('stage-readonly');
    container.querySelectorAll('input, textarea, select, button').forEach(el => {
      if (el.dataset.allowRead === '1') return;
      el.disabled = true;
    });
  }

  function getNextStage(detail, stage) {
    if (!detail || !Array.isArray(detail.stages) || !stage) return null;
    const ordered = detail.stages.slice().sort((a, b) => {
      if (a.position === b.position) return (a.id || 0) - (b.id || 0);
      return (a.position || 0) - (b.position || 0);
    });
    const idx = ordered.findIndex(s => s.id === stage.id);
    if (idx === -1) return null;
    return ordered[idx + 1] || null;
  }

  function renderStageCompletionHint(incidentId, detail, stage) {
    const wrap = document.createElement('div');
    wrap.className = 'stage-hint';
    const next = getNextStage(detail, stage);
    if (next) {
      const text = document.createElement('span');
      text.textContent = t('incidents.stage.completedNext');
      wrap.appendChild(text);
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn ghost';
      btn.textContent = t('incidents.stage.goToNext');
      btn.addEventListener('click', () => {
        detail.activeStageId = next.id;
        renderIncidentStages(incidentId);
      });
      wrap.appendChild(btn);
    } else {
      const text = document.createElement('span');
      text.textContent = t('incidents.stage.completedNoNext');
      wrap.appendChild(text);
      const addBtn = document.createElement('button');
      addBtn.type = 'button';
      addBtn.className = 'btn ghost';
      addBtn.textContent = t('incidents.addStage');
      addBtn.addEventListener('click', () => IncidentsPage.openStageModal(incidentId));
      wrap.appendChild(addBtn);
    }
    const closure = detail.stages.find(s => s.stageType === 'closure' && s.id !== stage.id);
    if (closure) {
      const goClosure = document.createElement('button');
      goClosure.type = 'button';
      goClosure.className = 'btn ghost';
      goClosure.textContent = t('incidents.stage.goToClosure');
      goClosure.addEventListener('click', () => {
        detail.activeStageId = closure.id;
        renderIncidentStages(incidentId);
      });
      wrap.appendChild(goClosure);
    }
    return wrap;
  }

  function renderStageOverview(incidentId, detail, container) {
    const grid = document.createElement('div');
    grid.className = 'stage-overview-grid';
    grid.appendChild(renderOverviewStatus(incidentId, detail));
    grid.appendChild(renderOverviewTimeline(incidentId));
    container.appendChild(grid);
    if (IncidentsPage.bindTimelineControls) {
      IncidentsPage.bindTimelineControls(incidentId);
    }
    if (IncidentsPage.ensureIncidentTimeline) {
      IncidentsPage.ensureIncidentTimeline(incidentId);
    }
  }

  function renderOverviewStageList(incidentId, detail) {
    const col = document.createElement('div');
    col.className = 'overview-col';
    const header = document.createElement('div');
    header.className = 'overview-header';
    const title = document.createElement('div');
    title.className = 'overview-title';
    title.textContent = t('incidents.overview.stageListTitle');
    header.appendChild(title);
    const hint = document.createElement('div');
    hint.className = 'form-hint';
    hint.textContent = t('incidents.stage.overviewHelper');
    header.appendChild(hint);
    col.appendChild(header);
    const list = document.createElement('div');
    list.className = 'overview-stage-list';
    (detail.stages || []).forEach(stage => {
      const card = document.createElement('div');
      card.className = 'overview-stage-card';
      if (detail.activeStageId === stage.id) {
        card.classList.add('active');
      }
      const row = document.createElement('div');
      row.className = 'overview-stage-row';
      const name = document.createElement('div');
      name.className = 'overview-stage-name';
      name.textContent = stage.is_default ? t('incidents.stage.overview') : stage.title;
      row.appendChild(name);
      const status = document.createElement('span');
      status.className = `pill stage-status-pill status-${stage.status || 'open'}`;
      status.textContent = t(`incidents.stage.status.${stage.status || 'open'}`);
      row.appendChild(status);
      card.appendChild(row);
      const actions = document.createElement('div');
      actions.className = 'overview-stage-actions';
      const openBtn = document.createElement('button');
      openBtn.type = 'button';
      openBtn.className = 'btn ghost';
      openBtn.textContent = t('incidents.overview.openStage');
      openBtn.addEventListener('click', () => {
        detail.activeStageId = stage.id;
        detail.activeInnerTab = 'stages';
        renderIncidentStages(incidentId);
      });
      actions.appendChild(openBtn);
      card.appendChild(actions);
      list.appendChild(card);
    });
    col.appendChild(list);
    return col;
  }

  function renderOverviewStatus(incidentId, detail) {
    const col = document.createElement('div');
    col.className = 'overview-col';
    const header = document.createElement('div');
    header.className = 'overview-header';
    const title = document.createElement('div');
    title.className = 'overview-title';
    title.textContent = t('incidents.overview.stageListTitle');
    header.appendChild(title);
    col.appendChild(header);
    const select = document.createElement('select');
    select.className = 'select status-select';
    const options = [
      { value: 'draft', label: t('incidents.status.draft') },
      { value: 'open', label: t('incidents.status.collect') },
      { value: 'in_progress', label: t('incidents.status.investigate') },
      { value: 'contained', label: t('incidents.status.respond') },
      { value: 'resolved', label: t('incidents.status.report') },
      { value: 'waiting', label: t('incidents.status.waiting') },
      { value: 'waiting_info', label: t('incidents.status.waiting_info') },
      { value: 'approval', label: t('incidents.status.approval') },
    ];
    const knownStatuses = new Set(options.map(o => o.value));
    options.forEach(opt => {
      const o = document.createElement('option');
      o.value = opt.value;
      o.textContent = opt.label;
      select.appendChild(o);
    });
    const currentStatus = (detail.incident?.status || 'draft').toLowerCase();
    if (currentStatus === 'closed') {
      const closedOpt = document.createElement('option');
      closedOpt.value = 'closed';
      closedOpt.textContent = t('incidents.status.closed');
      closedOpt.disabled = true;
      closedOpt.selected = true;
      select.appendChild(closedOpt);
      select.disabled = true;
    } else if (!knownStatuses.has(currentStatus)) {
      const currentOpt = document.createElement('option');
      currentOpt.value = currentStatus;
      currentOpt.textContent = t(`incidents.status.${currentStatus}`) || currentStatus;
      currentOpt.selected = true;
      select.appendChild(currentOpt);
    } else {
      select.value = currentStatus;
    }
    const saveBtn = document.createElement('button');
    saveBtn.type = 'button';
    saveBtn.className = 'btn primary';
    saveBtn.textContent = t('incidents.overview.saveStatus');
    saveBtn.disabled = detail.readOnly;
    select.disabled = detail.readOnly;
    select.addEventListener('change', () => {
      detail.statusDraft = select.value;
    });
    saveBtn.addEventListener('click', async () => {
      if (detail.statusSaving) return;
      const nextStatus = (select.value || '').toLowerCase();
      saveBtn.disabled = true;
      select.disabled = true;
      detail.statusSaving = true;
      const baseIncident = detail.incident ? { ...detail.incident } : null;
      try {
        let prevVersion = baseIncident?.version;
        if (incidentId) {
          try {
            const latest = await Api.get(`/api/incidents/${incidentId}`);
            const latestIncident = latest.incident || latest;
            if (latestIncident && latestIncident.version) {
              prevVersion = latestIncident.version;
            }
          } catch (_) {
            // Use local version fallback.
          }
        }
        if (baseIncident && prevVersion) {
          detail.incident = { ...baseIncident, status: nextStatus };
          syncIncident(detail.incident);
          if (IncidentsPage.updateIncidentTabTitle) {
            IncidentsPage.updateIncidentTabTitle(incidentId, detail.incident);
          }
          updateIncidentStatusUI(incidentId, nextStatus);
        }
        const res = await Api.put(`/api/incidents/${incidentId}`, {
          status: nextStatus,
          version: prevVersion
        });
        const updated = res.incident || res;
        detail.incident = updated;
        detail.readOnly = (updated.status || '').toLowerCase() === 'closed';
        detail.statusDraft = '';
        syncStageReadOnly(detail);
        syncIncident(updated);
        if (IncidentsPage.updateIncidentTabTitle) {
          IncidentsPage.updateIncidentTabTitle(incidentId, updated);
        }
        updateIncidentStatusUI(incidentId, (updated.status || nextStatus).toLowerCase());
        renderIncidentStages(incidentId);
      } catch (err) {
        const msg = (err && err.message ? err.message : '').trim();
        if (msg === 'incidents.conflictVersion') {
          try {
            const latest = await Api.get(`/api/incidents/${incidentId}`);
            const latestIncident = latest.incident || latest;
            const latestStatus = (latestIncident?.status || '').toLowerCase();
            if (latestIncident) {
              detail.incident = latestIncident;
              detail.readOnly = latestStatus === 'closed';
              syncStageReadOnly(detail);
              syncIncident(latestIncident);
              if (IncidentsPage.updateIncidentTabTitle) {
                IncidentsPage.updateIncidentTabTitle(incidentId, latestIncident);
              }
            }
            const uiStatus = latestStatus || nextStatus;
            detail.statusDraft = '';
            updateIncidentStatusUI(incidentId, uiStatus);
            renderIncidentStages(incidentId);
            // Conflict usually means parallel update; keep synchronized state without forced rollback.
            return;
          } catch (_) {
            // Suppress false-positive UI errors; status may already be applied server-side.
            return;
          }
        }
        // For non-conflict errors keep optimistic visual status and refresh from server asynchronously.
        setTimeout(async () => {
          try {
            const latest = await Api.get(`/api/incidents/${incidentId}`);
            const latestIncident = latest.incident || latest;
            if (!latestIncident) return;
            detail.incident = latestIncident;
            detail.readOnly = (latestIncident.status || '').toLowerCase() === 'closed';
            detail.statusDraft = '';
            syncStageReadOnly(detail);
            syncIncident(latestIncident);
            if (IncidentsPage.updateIncidentTabTitle) {
              IncidentsPage.updateIncidentTabTitle(incidentId, latestIncident);
            }
            updateIncidentStatusUI(incidentId, (latestIncident.status || nextStatus).toLowerCase());
            renderIncidentStages(incidentId);
          } catch (_) {}
        }, 250);
        // Do not show blocking popup for status save flow:
        // the server state may already be updated and UI will be re-synced asynchronously.
      } finally {
        detail.statusSaving = false;
        saveBtn.disabled = false;
        select.disabled = detail.readOnly;
      }
    });
    col.appendChild(select);
    col.appendChild(saveBtn);
    return col;
  }

  function updateIncidentStatusUI(incidentId, status) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    if (!panel || !status) return;
    const state = IncidentsPage.state;
    const detail = state?.incidentDetails?.get(incidentId);
    const display = detail?.incident && IncidentsPage.getIncidentStatusDisplay
      ? IncidentsPage.getIncidentStatusDisplay(detail.incident)
      : { status, label: t(`incidents.status.${status}`) };
    const pill = panel.querySelector('.incident-status-row .status-pill');
    if (pill) {
      pill.className = `pill status-pill status-${display.status || status}`;
      pill.textContent = display.label || display.status || status;
    }
    const statusSelect = panel.querySelector('.status-select');
    if (statusSelect) {
      const selectStatus = (detail?.incident?.status || status || '').toLowerCase();
      statusSelect.value = selectStatus;
    }
    if (state && Array.isArray(state.incidents)) {
      const idx = state.incidents.findIndex(i => i.id === incidentId);
      if (idx !== -1) {
        const updatedAt = detail?.incident?.updated_at || state.incidents[idx].updated_at;
        state.incidents[idx] = { ...state.incidents[idx], status, updated_at: updatedAt };
      }
    }
    if (state && state.dashboard) {
      const updateItems = (items) => {
        if (!Array.isArray(items)) return;
        items.forEach(item => {
          if (item && item.id === incidentId) {
            item.status = status;
          }
        });
      };
      updateItems(state.dashboard.mine);
      updateItems(state.dashboard.attention);
      updateItems(state.dashboard.recent);
    }
    if (IncidentsPage.renderTableRows) {
      IncidentsPage.renderTableRows();
    }
    if (IncidentsPage.renderHome) {
      IncidentsPage.renderHome();
    }
  }

  function renderOverviewTimeline(incidentId) {
    const col = document.createElement('div');
    col.className = 'overview-col';
    const header = document.createElement('div');
    header.className = 'overview-header';
    const title = document.createElement('div');
    title.className = 'overview-title';
    title.textContent = t('incidents.overview.timelineTitle');
    header.appendChild(title);
    col.appendChild(header);
    const wrap = document.createElement('div');
    wrap.className = 'overview-timeline-wrap';
    wrap.innerHTML = IncidentsPage.buildTimelineLayout ? IncidentsPage.buildTimelineLayout({ scope: 'overview' }) : '';
    const timeline = wrap.querySelector('.incident-timeline');
    if (timeline) {
      col.appendChild(timeline);
    } else {
      const list = document.createElement('div');
      list.className = 'timeline-list';
      list.dataset.overview = '1';
      col.appendChild(list);
    }
    return col;
  }

  async function completeStage(incidentId, stageId) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail) return;
    const stage = detail.stages.find(s => s.id === stageId);
    if (!stage || stage.readOnly || stage.is_default) return;
    if (isStageDirty(stage)) {
      const saved = await saveStageContent(incidentId, stage, { silent: false });
      if (!saved) return;
    }
    try {
      const res = await Api.post(`/api/incidents/${incidentId}/stages/${stageId}/complete`);
      stage.status = (res.status || 'done').toLowerCase();
      stage.closed_at = res.closed_at;
      stage.closed_by = res.closed_by;
      stage.readOnly = true;
      stage.initialSerialized = stage.currentSerialized || stage.initialSerialized;
      const next = getNextStage(detail, stage);
      if (next) {
        detail.activeStageId = next.id;
      }
      if (IncidentsPage.renderIncidentPanel) {
        IncidentsPage.renderIncidentPanel(incidentId);
      } else {
        renderIncidentStages(incidentId);
      }
      if (IncidentsPage.updateIncidentSaveState) IncidentsPage.updateIncidentSaveState(incidentId);
    } catch (err) {
      showError(err, 'incidents.stageCompleteFailed');
    }
  }

  IncidentsPage.renderIncidentStages = renderIncidentStages;
  IncidentsPage.openStageModal = openStageModal;
  IncidentsPage.bindStageModal = bindStageModal;
  IncidentsPage.submitStageModal = submitStageModal;
  IncidentsPage.closeStageModal = closeStageModal;
  IncidentsPage.requestCloseStage = requestCloseStage;
  IncidentsPage.saveStageContent = saveStageContent;
  IncidentsPage.saveDirtyStages = saveDirtyStages;
  IncidentsPage.isStageDirty = isStageDirty;
})();
