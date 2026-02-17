(() => {
  const state = IncidentsPage.state;
  const { t, showError, escapeHtml } = IncidentsPage;
  const TAG_FALLBACK = [
    { code: 'COMMERCIAL_SECRET', label: 'Коммерческая тайна' },
    { code: 'PERSONAL_DATA', label: 'ПДн' },
    { code: 'CRITICAL_INFRASTRUCTURE', label: 'КИИ' },
    { code: 'FEDERAL_LAW_152', label: 'ФЗ 152' },
    { code: 'FEDERAL_LAW_149', label: 'ФЗ 149' },
    { code: 'FEDERAL_LAW_187', label: 'ФЗ 187' },
    { code: 'FEDERAL_LAW_63', label: 'ФЗ 63' },
    { code: 'PCI_DSS', label: 'PCI DSS' },
  ];

  async function openIncidentTab(incidentId, options = {}) {
    const existing = state.tabs.find(t => t.type === 'incident' && t.incidentId === incidentId);
    if (existing) {
      IncidentsPage.switchTab(existing.id);
      return;
    }
    const incident = state.incidents.find(i => i.id === incidentId);
    const tabId = `incident-${incidentId}`;
    state.tabs.push({
      id: tabId,
      type: 'incident',
      title: incident ? `${t('incidents.tabs.incidentPrefix')} ${IncidentsPage.incidentLabel(incident)}` : `${t('incidents.tabs.incidentPrefix')}`,
      closable: true,
      incidentId
    });
    const panels = document.getElementById('incidents-panels');
    const panel = document.createElement('div');
    panel.className = 'tab-panel';
    panel.dataset.tab = tabId;
    panel.id = `panel-${tabId}`;
    panels.appendChild(panel);
    if (IncidentsPage.renderTabBar) {
      IncidentsPage.renderTabBar();
    }
    await ensureIncidentDetails(incidentId);
    const detail = state.incidentDetails.get(incidentId);
    if (!detail || (!detail.incident && !detail.errorKey)) {
      IncidentsPage.removeTab(tabId);
      return;
    }
    renderIncidentPanel(incidentId);
    IncidentsPage.switchTab(tabId);
  }

  async function ensureIncidentDetails(incidentId) {
    if (state.incidentDetails.has(incidentId)) return;
    state.incidentDetails.set(incidentId, {
      loading: true,
      stages: [],
      activeStageId: null,
      incident: null,
      readOnly: false,
      activeInnerTab: 'stages',
      participants: [],
      people: { owner: '', assignee: '', participants: [] },
      peopleInitial: { owner: '', assignee: '', participants: [] },
      peopleDirty: false,
      peopleSaving: false,
      links: [],
      controlLinks: [],
      attachments: [],
      timeline: [],
      timelineFilter: ''
    });
    renderIncidentPanel(incidentId);
    try {
      const res = await Api.get(`/api/incidents/${incidentId}`);
      const incident = res.incident || res;
      const participants = res.participants || [];
      const incidentReadOnly = (incident.status || '').toLowerCase() === 'closed';
      const stagesRes = await Api.get(`/api/incidents/${incidentId}/stages`);
      const stages = stagesRes.items || [];
      const stageDetails = await Promise.all(stages.map(async stage => {
        const entry = await Api.get(`/api/incidents/${incidentId}/stages/${stage.id}/content`);
        const parsed = IncidentsPage.StageBlocks
          ? IncidentsPage.StageBlocks.parseContent(entry.content || '')
          : { stageType: 'custom', blocks: [], serialized: entry.content || '' };
        const stageType = stage.is_default ? 'overview' : (parsed.stageType || 'custom');
        const status = (stage.status || 'open').toLowerCase();
        const readOnly = incidentReadOnly || status === 'done';
        return {
          id: stage.id,
          title: stage.title,
          is_default: !!stage.is_default,
          closable: !stage.is_default,
          position: stage.position,
          version: stage.version,
          entryVersion: entry.version || 1,
          status,
          closed_at: stage.closed_at,
          closed_by: stage.closed_by,
          readOnly,
          stageType,
          blocks: parsed.blocks || [],
          initialSerialized: parsed.serialized || '',
          currentSerialized: parsed.serialized || '',
        };
      }));
      const detail = state.incidentDetails.get(incidentId);
      if (!detail) return;
      detail.loading = false;
      detail.incident = incident;
       detail.readOnly = incidentReadOnly;
      detail.participants = participants;
      detail.people = buildPeopleState(incident, participants);
      detail.peopleInitial = clonePeople(detail.people);
      detail.peopleDirty = false;
      detail.stages = stageDetails;
      detail.activeStageId = stageDetails[0]?.id || null;
      syncIncident(incident);
      updateIncidentTabTitle(incidentId, incident);
      renderIncidentPanel(incidentId);
      loadControlLinks(incidentId);
    } catch (err) {
      const raw = (err && err.message ? err.message : '').trim();
      if (raw === 'incidents.forbidden' || raw === 'incidents.notFound' || raw === 'incidents.deleted') {
        closeIncidentContext(incidentId);
        showError(err, 'incidents.notFound');
        return;
      }
      const detail = state.incidentDetails.get(incidentId);
      if (detail) {
        detail.loading = false;
      }
      showError(err, 'incidents.notFound');
    }
  }

  function renderIncidentPanel(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    if (!detail || detail.loading) {
      panel.innerHTML = '';
      return;
    }
    if (detail.errorKey) {
      panel.innerHTML = `
        <div class="card incident-card">
          <div class="card-body">
            <div class="alert">
              <strong>${escapeHtml(t('incidents.accessDeniedTitle'))}</strong>
              <div>${escapeHtml(t('incidents.accessDeniedBody'))}</div>
            </div>
          </div>
        </div>`;
      return;
    }
    const incident = detail.incident || state.incidents.find(i => i.id === incidentId);
    const meta = incident?.meta || {};
    const caseSLA = incident?.case_sla || {};
    if (!incident) return;
    const statusDisplay = IncidentsPage.getIncidentStatusDisplay
      ? IncidentsPage.getIncidentStatusDisplay(incident)
      : { status: incident.status, label: t(`incidents.status.${incident.status}`) };
    panel.innerHTML = `
      <div class="card incident-card ${detail.readOnly ? 'incident-readonly' : ''}">
        <div class="incident-stage-bar">
          <div class="tabs stage-tabs incident-stage-tabs"></div>
          <div class="stage-actions">
            <button class="btn ghost incident-add-stage" data-incident="${incidentId}">${t('incidents.addStage')}</button>
          </div>
        </div>
        <div class="card-header incident-header">
          <div>
            <h3>${t('incidents.incidentTitle')} ${IncidentsPage.incidentLabel(incident)}</h3>
            <p>${escapeHtml(incident.title)} - ${escapeHtml(t(`incidents.severity.${incident.severity}`))}</p>
            <div class="pill-row incident-status-row">
              <span class="pill status-pill status-${escapeHtml(statusDisplay.status)}">${escapeHtml(statusDisplay.label)}</span>
              <span class="pill subtle">${t('incidents.createdAtShort')}: ${escapeHtml(IncidentsPage.formatDate(incident.created_at))}</span>
              ${incident.closed_at ? `<span class="pill subtle">${t('incidents.closedAtShort')}: ${escapeHtml(IncidentsPage.formatDate(incident.closed_at))}</span>` : ''}
            </div>
          </div>
          <div class="actions">
            <button class="btn ghost incident-close">${t('incidents.closeIncident')}</button>
            ${IncidentsPage.hasPermission && IncidentsPage.hasPermission('reports.create') ? `<button class="btn ghost incident-create-report">${t('incidents.report.createReport')}</button>` : ''}
            <button class="btn primary incident-save">${t('incidents.form.save')}</button>
          </div>
        </div>
        <div class="card-body">
          <div class="incident-passport">
            <div class="incident-meta">
              <div class="meta-field">
                <label>${t('incidents.classification.level')}</label>
                <select class="incident-classification-level"></select>
              </div>
              <div class="meta-field wide">
                <label>${t('incidents.classification.tags')}</label>
                <select class="incident-classification-tags select" multiple></select>
                <div class="selected-hint" data-tag-hint="incident-classification-tags"></div>
              </div>
              <div class="meta-actions">
                <button class="btn ghost incident-classification-save">${t('incidents.classification.save')}</button>
              </div>
            </div>
            <div class="incident-meta incident-overview">
              <div class="meta-field">
                <label>${t('incidents.form.incidentType')}</label>
                <div class="meta-value">${formatMetaValue(meta.incident_type)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.detectionSource')}</label>
                <div class="meta-value">${formatMetaValue(meta.detection_source)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.source')}</label>
                <div class="meta-value">${renderIncidentSource(incident)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.sla')}</label>
                <div class="meta-value">${formatMetaValue(meta.sla_response)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.slaDeadline')}</label>
                <div class="meta-value">${formatMetaValue(meta.first_response_deadline, { type: 'datetime' })}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.resolveDeadline')}</label>
                <div class="meta-value">${formatMetaValue(meta.resolve_deadline, { type: 'datetime' })}</div>
              </div>
              <div class="meta-field wide">
                <label>${t('incidents.form.slaState')}</label>
                <div class="meta-value">${renderSlaState(caseSLA)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.assets')}</label>
                <div class="meta-value">${formatMetaValue(meta.assets)}</div>
              </div>
              <div class="meta-field wide">
                <label>${t('incidents.form.tags')}</label>
                <div class="meta-value">${renderMetaTags(meta.tags)}</div>
              </div>
              <div class="meta-field wide">
                <label>${t('incidents.controls')}</label>
                <div class="meta-value">${renderControlLinks(detail.controlLinks)}</div>
              </div>
            </div>
            <div class="incident-meta incident-people">
              <div class="meta-field">
                <label>${t('incidents.form.owner')}</label>
                <select class="incident-user-select incident-owner"></select>
                <div class="selected-hint incident-owner-hint"></div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.performers')}</label>
                <select multiple class="incident-user-select incident-assignee"></select>
                <div class="selected-hint incident-assignee-hint"></div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.watchers')}</label>
                <select multiple class="incident-user-select incident-participants"></select>
                <div class="selected-hint incident-participants-hint"></div>
              </div>
            </div>
            <div class="incident-meta incident-notes">
              <div class="meta-field wide">
                <label>${t('incidents.form.description')}</label>
                <div class="meta-value">${formatMetaValue(incident.description)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.whatHappened')}</label>
                <div class="meta-value">${formatMetaValue(meta.what_happened)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.detectedAt')}</label>
                <div class="meta-value">${formatMetaValue(meta.detected_at, { type: 'datetime' })}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.affected')}</label>
                <div class="meta-value">${formatMetaValue(meta.affected_systems)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.risks')}</label>
                <div class="meta-value">${formatMetaValue(meta.risk)}</div>
              </div>
              <div class="meta-field">
                <label>${t('incidents.form.actions')}</label>
                <div class="meta-value">${formatMetaValue(meta.actions_taken)}</div>
              </div>
              <div class="meta-field wide">
                <label>${t('incidents.form.postmortem')}</label>
                <textarea class="input incident-postmortem-text" rows="4" ${detail.readOnly ? 'disabled' : ''}>${escapeHtml(meta.postmortem || '')}</textarea>
                <div class="form-actions form-actions-inline">
                  <button class="btn ghost incident-postmortem-save" ${detail.readOnly ? 'disabled' : ''}>${t('incidents.form.postmortemSave')}</button>
                </div>
              </div>
            </div>
          </div>
          <div class="tabs inner-tabs incident-inner-tabs"></div>
          <div class="incident-inner-content"></div>
        </div>
      </div>`;
    const addBtn = panel.querySelector('.incident-add-stage');
    if (addBtn) {
      addBtn.addEventListener('click', () => IncidentsPage.openStageModal(incidentId));
    }
    const saveBtn = panel.querySelector('.incident-save');
    if (saveBtn) {
      saveBtn.addEventListener('click', async () => {
        await saveIncidentChanges(incidentId);
      });
    }
    const closeBtn = panel.querySelector('.incident-close');
    if (closeBtn) {
      const closeState = canCloseIncident(detail);
      closeBtn.disabled = !closeState.ok;
      closeBtn.title = closeState.reason || '';
      closeBtn.addEventListener('click', () => confirmCloseIncident(incidentId));
    }
    const postmortemBtn = panel.querySelector('.incident-postmortem-save');
    if (postmortemBtn) {
      postmortemBtn.addEventListener('click', () => savePostmortem(incidentId));
    }
    renderIncidentClassification(incidentId);
    renderIncidentPeople(incidentId);
    renderIncidentInnerTabs(incidentId);
    renderIncidentInnerContent(incidentId);
    syncStageReadOnly(detail);
    applyIncidentReadOnly(panel, detail);
    IncidentsPage.renderIncidentStages(incidentId);
    updateIncidentSaveState(incidentId);
  }

  function syncStageReadOnly(detail) {
    if (!detail || !Array.isArray(detail.stages)) return;
    detail.stages.forEach(stage => {
      const status = (stage.status || 'open').toLowerCase();
      stage.status = status;
      stage.readOnly = detail.readOnly || status === 'done';
    });
  }

  function applyIncidentReadOnly(panel, detail) {
    if (!panel || !detail) return;
    const readOnly = !!detail.readOnly;
    panel.classList.toggle('incident-readonly', readOnly);
    const addBtn = panel.querySelector('.incident-add-stage');
    if (addBtn) addBtn.disabled = readOnly;
    const saveBtn = panel.querySelector('.incident-save');
    if (saveBtn && readOnly) saveBtn.disabled = true;
    const classSave = panel.querySelector('.incident-classification-save');
    if (classSave) classSave.disabled = readOnly;
    if (readOnly) {
      panel.querySelectorAll('.incident-passport select, .incident-passport input, .incident-passport textarea, .incident-passport button').forEach(el => {
        if (el.classList.contains('incident-close')) return;
        el.disabled = true;
      });
    }
  }

  function stageHasDecisionBlock(stage) {
    if (!stage || stage.stageType !== 'closure') return false;
    if (!Array.isArray(stage.blocks)) return false;
    return stage.blocks.some(block => block && block.type === 'decisions' && Array.isArray(block.items) && block.items.length > 0);
  }

  function findClosureStage(detail) {
    if (!detail || !Array.isArray(detail.stages)) return null;
    return detail.stages.find(s => s.stageType === 'closure') || null;
  }

  function canCloseIncident(detail) {
    if (!detail || detail.readOnly) {
      return { ok: false, reason: detail?.readOnly ? t('incidents.close.reasonClosed') : t('incidents.close.reasonUnavailable') };
    }
    const closureStage = findClosureStage(detail);
    if (!closureStage) {
      return { ok: false, reason: t('incidents.close.missingClosure') };
    }
    if ((closureStage.status || 'open').toLowerCase() !== 'done') {
      return { ok: false, reason: t('incidents.close.stageNotDone') };
    }
    if (!stageHasDecisionBlock(closureStage)) {
      return { ok: false, reason: t('incidents.close.noDecisions') };
    }
    return { ok: true, stage: closureStage };
  }

  async function confirmCloseIncident(incidentId) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail) return;
    let availability = canCloseIncident(detail);
    if (!availability.ok) return;
    const ok = await IncidentsPage.confirmAction({
      title: t('incidents.closeIncident'),
      message: t('incidents.closeConfirm'),
      confirmText: t('incidents.closeIncident'),
      cancelText: t('common.cancel'),
    });
    if (!ok) return;
    if (IncidentsPage.saveDirtyStages) {
      const saved = await IncidentsPage.saveDirtyStages(incidentId, { silent: false });
      if (saved === false && availability.stage && IncidentsPage.isStageDirty && IncidentsPage.isStageDirty(availability.stage)) {
        showError({ message: 'incidents.conflictVersion' }, 'incidents.conflictVersion');
        return;
      }
    }
    availability = canCloseIncident(detail);
    if (!availability.ok) {
      showError({ message: availability.reason || 'incidents.close.reasonUnavailable' }, 'incidents.close.reasonUnavailable');
      return;
    }
    try {
      const res = await Api.post(`/api/incidents/${incidentId}/close`);
      const updated = res.incident || res;
      detail.incident = updated;
      detail.readOnly = true;
      syncIncident(updated);
      syncStageReadOnly(detail);
      updateIncidentTabTitle(incidentId, updated);
      renderIncidentPanel(incidentId);
    } catch (err) {
      showError(err, 'incidents.closeFailed');
    }
  }

  function renderIncidentInnerTabs(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    const tabs = panel.querySelector('.incident-inner-tabs');
    if (!tabs) return;
    const items = [
      { id: 'stages', label: t('incidents.inner.stages') },
      { id: 'timeline', label: t('incidents.inner.timeline') },
      { id: 'links', label: t('incidents.inner.links') },
    ];
    if (!detail.activeInnerTab || !items.some(item => item.id === detail.activeInnerTab)) {
      detail.activeInnerTab = 'stages';
    }
    tabs.innerHTML = '';
    items.forEach(item => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'tab-btn';
      if (detail.activeInnerTab === item.id) btn.classList.add('active');
      btn.textContent = item.label;
      btn.addEventListener('click', () => {
        detail.activeInnerTab = item.id;
        renderIncidentInnerTabs(incidentId);
        renderIncidentInnerContent(incidentId);
      });
      tabs.appendChild(btn);
    });
  }

  function buildTimelineLayout(opts = {}) {
    const scope = opts.scope ? ` data-scope="${opts.scope}"` : '';
    return `
      <div class="incident-timeline"${scope}>
        <div class="timeline-pane timeline-compose">
          <div class="form-grid two-column">
            <div class="form-field">
              <label>${t('incidents.stage.blocks.timeline.type')}</label>
              <select class="select incident-timeline-type"></select>
              <input class="input incident-timeline-type-custom" hidden placeholder="${t('incidents.timeline.typeCustom')}">
            </div>
            <div class="form-field">
              <label>${t('incidents.stage.blocks.timeline.message')}</label>
              <textarea class="textarea incident-timeline-message" placeholder="${t('incidents.stage.blocks.timeline.messagePlaceholder')}"></textarea>
            </div>
          </div>
          <div class="form-actions form-actions-inline">
            <button class="btn ghost incident-timeline-refresh">${t('incidents.timeline.refresh')}</button>
            <button class="btn primary incident-timeline-save">${t('incidents.timeline.add')}</button>
          </div>
        </div>
        <div class="timeline-pane timeline-events">
          <div class="timeline-list-header">
            <select class="select incident-timeline-filter"></select>
          </div>
          <div class="timeline-list"></div>
        </div>
      </div>`;
  }

  function renderIncidentInnerContent(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    const content = panel.querySelector('.incident-inner-content');
    if (!content) return;
    const active = detail.activeInnerTab || 'stages';
    const passport = panel.querySelector('.incident-passport');
    if (passport) passport.hidden = false;
    if (active === 'stages') {
      content.innerHTML = `
        <div class="incident-stage-content"></div>`;
      IncidentsPage.renderIncidentStages(incidentId);
      return;
    }
    if (active === 'links') {
      content.innerHTML = `
        <div class="incident-links">
          <div class="form-grid two-column">
            <div class="form-field">
              <label>${t('incidents.links.type')}</label>
              <select class="select incident-link-type">
                <option value="doc">${t('incidents.links.type.doc')}</option>
                <option value="incident">${t('incidents.links.type.incident')}</option>
                <option value="report">${t('incidents.links.type.report')}</option>
                <option value="other">${t('incidents.links.type.other')}</option>
              </select>
            </div>
            <div class="form-field">
              <label>${t('incidents.links.reference')}</label>
              <select class="select incident-link-target"></select>
            </div>
          </div>
          <div class="form-grid two-column">
            <div class="form-field">
              <label>${t('incidents.links.comment')}</label>
              <input class="input incident-link-comment" placeholder="${t('incidents.links.commentPlaceholder')}">
            </div>
            <div class="form-actions form-actions-inline">
              <button class="btn primary incident-link-add">${t('incidents.links.add')}</button>
            </div>
          </div>
          <div class="table-responsive">
            <table class="data-table compact">
              <thead>
                <tr>
                  <th>${t('incidents.links.type')}</th>
                  <th>${t('incidents.links.title')}</th>
                  <th>${t('incidents.links.comment')}</th>
                  <th>${t('incidents.links.status')}</th>
                  <th>${t('incidents.links.actions')}</th>
                </tr>
              </thead>
              <tbody class="incident-links-body"></tbody>
            </table>
          </div>
        </div>`;
      IncidentsPage.bindLinkControls(incidentId);
      IncidentsPage.ensureIncidentLinks(incidentId);
      return;
    }
    if (active === 'timeline') {
      content.innerHTML = buildTimelineLayout({ scope: 'timeline-tab' });
      IncidentsPage.bindTimelineControls(incidentId);
      IncidentsPage.ensureIncidentTimeline(incidentId);
      return;
    }
    // Timeline and export tabs are hidden; keep logic in overview only when needed.
  }

  function renderIncidentClassification(incidentId, preselected = null) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail || !detail.incident) return;
    const select = panel.querySelector('.incident-classification-level');
    const tagsWrap = panel.querySelector('.incident-classification-tags');
    const saveBtn = panel.querySelector('.incident-classification-save');
    if (!select || !tagsWrap || !saveBtn) return;
    if (!tagsWrap.id) tagsWrap.id = `incident-classification-tags-${incidentId}`;
    const levels = (typeof ClassificationDirectory !== 'undefined' && ClassificationDirectory.codes)
      ? ClassificationDirectory.codes()
      : ['PUBLIC', 'INTERNAL', 'CONFIDENTIAL', 'RESTRICTED', 'SECRET', 'TOP_SECRET', 'SPECIAL_IMPORTANCE'];
    select.innerHTML = '';
    select.classList.add('select');
    levels.forEach(code => {
      const opt = document.createElement('option');
      opt.value = code;
      opt.textContent = t(`docs.classification.${code.toLowerCase()}`) || code;
      select.appendChild(opt);
    });
    const levelIdx = typeof detail.incident.classification_level === 'number' ? detail.incident.classification_level : 1;
    select.value = levels[levelIdx] || 'INTERNAL';
    const tagList = incidentTags();
    const currentTags = (preselected && preselected.length)
      ? preselected.map(t => (t || '').toUpperCase())
      : (detail.incident.classification_tags || []).map(t => t.toUpperCase());
    tagsWrap.innerHTML = '';
    tagList.forEach(tag => {
      const code = tag.code || tag;
      const opt = document.createElement('option');
      opt.value = code;
      opt.textContent = tagLabel(code);
      opt.dataset.label = opt.textContent;
      opt.selected = currentTags.includes((code || '').toUpperCase());
      tagsWrap.appendChild(opt);
    });
    if (typeof DocsPage !== 'undefined' && DocsPage.enhanceMultiSelects && tagsWrap.id) {
      DocsPage.enhanceMultiSelects([tagsWrap.id]);
    }
    if (typeof DocUI !== 'undefined' && DocUI.bindTagHint) {
      const hint = panel.querySelector('[data-tag-hint="incident-classification-tags"]');
      if (hint) hint.dataset.tagHint = tagsWrap.id;
      DocUI.bindTagHint(tagsWrap, hint);
    }
    if (!tagsWrap.dataset.tagsBound) {
      document.addEventListener('tags:changed', () => {
        const selectedNow = Array.from(tagsWrap.selectedOptions || []).map(i => i.value);
        renderIncidentClassification(incidentId, selectedNow);
      });
      tagsWrap.dataset.tagsBound = '1';
    }
    saveBtn.onclick = async () => {
      const selectedTags = Array.from(tagsWrap.selectedOptions || []).map(i => i.value);
      try {
        const res = await Api.put(`/api/incidents/${incidentId}`, {
          classification_level: select.value,
          classification_tags: selectedTags,
          version: detail.incident.version
        });
        detail.incident = res;
        syncIncident(res);
        renderIncidentClassification(incidentId);
      } catch (err) {
        showError(err, 'incidents.classification.saveFailed');
      }
    };
  }

  function buildPeopleState(incident, participants) {
    const ownerUser = incident?.owner_user_id && typeof UserDirectory !== 'undefined'
      ? UserDirectory.get(incident.owner_user_id)
      : null;
    const assigneeUser = incident?.assignee_user_id && typeof UserDirectory !== 'undefined'
      ? UserDirectory.get(incident.assignee_user_id)
      : null;
    const owner = ownerUser ? ownerUser.username : (incident?.owner || '');
    const assignee = assigneeUser ? assigneeUser.username : '';
    const participantList = (participants || [])
      .map(p => p.username)
      .filter(Boolean);
    return { owner, assignee, participants: participantList };
  }

  function clonePeople(people) {
    return {
      owner: people.owner || '',
      assignee: people.assignee || '',
      participants: (people.participants || []).slice()
    };
  }

  function normalizeList(list) {
    return Array.from(new Set((list || []).filter(Boolean))).sort();
  }

  function isSamePeople(a, b) {
    if (!a || !b) return false;
    if ((a.owner || '') !== (b.owner || '')) return false;
    if ((a.assignee || '') !== (b.assignee || '')) return false;
    return JSON.stringify(normalizeList(a.participants)) === JSON.stringify(normalizeList(b.participants));
  }

  function renderIncidentPeople(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    const ownerSel = panel.querySelector('.incident-owner');
    const assigneeSel = panel.querySelector('.incident-assignee');
    const participantsSel = panel.querySelector('.incident-participants');
    const ownerHint = panel.querySelector('.incident-owner-hint');
    const assigneeHint = panel.querySelector('.incident-assignee-hint');
    const participantsHint = panel.querySelector('.incident-participants-hint');
    if (!ownerSel || !assigneeSel || !participantsSel) return;
    [ownerSel, assigneeSel, participantsSel].forEach(sel => sel.classList.add('select'));
    if (!ownerSel.id) ownerSel.id = `incident-owner-${incidentId}`;
    if (!assigneeSel.id) assigneeSel.id = `incident-assignee-${incidentId}`;
    if (!participantsSel.id) participantsSel.id = `incident-participants-${incidentId}`;
    if (IncidentsPage.populateUserSelect) {
      IncidentsPage.populateUserSelect(ownerSel, detail.people.owner ? [detail.people.owner] : []);
      IncidentsPage.populateUserSelect(assigneeSel, detail.people.assignee ? [detail.people.assignee] : []);
      IncidentsPage.populateUserSelect(participantsSel, detail.people.participants || []);
    }
    if (IncidentsPage.enforceSingleSelect) IncidentsPage.enforceSingleSelect(ownerSel);
    if (IncidentsPage.enforceSingleSelect) IncidentsPage.enforceSingleSelect(assigneeSel);
    if (IncidentsPage.renderSelectedHint) {
      IncidentsPage.renderSelectedHint(ownerSel, ownerHint);
      IncidentsPage.renderSelectedHint(assigneeSel, assigneeHint);
      IncidentsPage.renderSelectedHint(participantsSel, participantsHint);
    }
    if (IncidentsPage.setDefaultSelectValues) {
      IncidentsPage.setDefaultSelectValues(ownerSel);
      IncidentsPage.setDefaultSelectValues(assigneeSel);
      IncidentsPage.setDefaultSelectValues(participantsSel);
    }
    const refresh = () => {
      detail.people = {
        owner: IncidentsPage.getSelectedValues ? (IncidentsPage.getSelectedValues(ownerSel)[0] || '') : '',
        assignee: IncidentsPage.getSelectedValues ? (IncidentsPage.getSelectedValues(assigneeSel)[0] || '') : '',
        participants: IncidentsPage.getSelectedValues ? IncidentsPage.getSelectedValues(participantsSel) : []
      };
      detail.peopleDirty = !isSamePeople(detail.people, detail.peopleInitial);
      updateIncidentSaveState(incidentId);
    };
    const onSelectChange = (sel) => {
      if (IncidentsPage.renderSelectedHint) {
        const hint = sel === assigneeSel ? assigneeHint : (sel === ownerSel ? ownerHint : participantsHint);
        IncidentsPage.renderSelectedHint(sel, hint);
      }
      refresh();
    };
    [ownerSel, assigneeSel, participantsSel].forEach(sel => {
      sel.addEventListener('change', () => onSelectChange(sel));
      sel.addEventListener('selectionrefresh', () => onSelectChange(sel));
    });
    refresh();
  }

  async function saveIncidentPeople(incidentId, opts = {}) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail || detail.readOnly || detail.statusSaving || !detail.peopleDirty || detail.peopleSaving) return false;
    if (!detail.incident) return false;
    detail.peopleSaving = true;
    const combinedParticipants = normalizeList(detail.people.participants || []);
    const assignee = detail.people.assignee || '';
    const owner = detail.people.owner || '';
    try {
      const res = await Api.put(`/api/incidents/${incidentId}`, {
        owner,
        assignee,
        participants: combinedParticipants,
        version: detail.incident.version
      });
      detail.incident = res;
      detail.peopleInitial = clonePeople(detail.people);
      detail.peopleDirty = false;
      syncIncident(res);
      updateIncidentSaveState(incidentId);
      return true;
    } catch (err) {
      if (!opts.silent) {
        showError(err, 'incidents.conflictVersion');
      }
      return false;
    } finally {
      detail.peopleSaving = false;
    }
  }

  async function saveIncidentChanges(incidentId, opts = {}) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail || detail.readOnly || detail.statusSaving) return;
    const tasks = [];
    if (IncidentsPage.saveDirtyStages) {
      tasks.push(IncidentsPage.saveDirtyStages(incidentId, { silent: !!opts.silent }));
    }
    if (detail.peopleDirty) {
      tasks.push(saveIncidentPeople(incidentId, { silent: !!opts.silent }));
    }
    if (!tasks.length) return;
    await Promise.all(tasks);
  }

  function updateIncidentSaveState(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail) return;
    const btn = panel.querySelector('.incident-save');
    if (!btn) return;
    btn.disabled = detail.readOnly || !isIncidentDirty(detail);
  }

  function isIncidentDirty(detail) {
    if (!detail || detail.readOnly) return false;
    const stageDirty = detail.stages.some(IncidentsPage.isStageDirty);
    return stageDirty || detail.peopleDirty;
  }

  function updateIncidentTabTitle(incidentId, incident) {
    const tab = state.tabs.find(t => t.type === 'incident' && t.incidentId === incidentId);
    if (!tab || !incident) return;
    tab.title = `${t('incidents.tabs.incidentPrefix')} ${IncidentsPage.incidentLabel(incident)}`;
    if (IncidentsPage.renderTabBar) IncidentsPage.renderTabBar();
  }

  function syncIncident(incident) {
    if (!incident) return;
    const idx = state.incidents.findIndex(i => i.id === incident.id);
    if (idx === -1) {
      state.incidents.push(incident);
    } else {
      state.incidents[idx] = { ...state.incidents[idx], ...incident };
    }
    if (IncidentsPage.renderHome) IncidentsPage.renderHome();
    if (IncidentsPage.renderTableRows) IncidentsPage.renderTableRows();
    if (IncidentsPage.loadDashboard) IncidentsPage.loadDashboard();
  }

  async function loadControlLinks(incidentId) {
    const detail = state.incidentDetails.get(incidentId);
    if (!detail) return;
    try {
      const res = await Api.get(`/api/incidents/${incidentId}/control-links`);
      detail.controlLinks = res.items || [];
    } catch (_) {
      detail.controlLinks = [];
    }
    renderIncidentPanel(incidentId);
  }

  function formatMetaValue(value, opts = {}) {
    const text = (value || '').toString().trim();
    if (!text) return `<span class="meta-empty">-</span>`;
    if (opts.type === 'datetime') {
      const formatted = IncidentsPage.formatDate(value);
      if (!formatted) return `<span class="meta-empty">-</span>`;
      return escapeHtml(formatted);
    }
    return escapeHtml(text).replace(/\n/g, '<br>');
  }

  function renderMetaTags(tags) {
    if (!tags || !tags.length) return `<span class="meta-empty">-</span>`;
    return tags.map(tag => `<span class="tag">${escapeHtml(tagLabel(tag))}</span>`).join(' ');
  }

  function renderControlLinks(items) {
    if (!items || !items.length) return `<span class="meta-empty">-</span>`;
    return items.map(item => {
      const label = `${escapeHtml(item.code)} - ${escapeHtml(item.title)}`;
      const rel = item.relation_type ? ` <span class="muted">(${escapeHtml(t(`controls.links.relation.${item.relation_type}`))})</span>` : '';
      return `<a class="tag" href="/controls?control=${encodeURIComponent(item.control_id)}">${label}</a>${rel}`;
    }).join(' ');
  }

  function renderIncidentSource(incident) {
    if (!incident || !incident.source) return `<span class="meta-empty">-</span>`;
    const source = (incident.source || '').toLowerCase();
    if (source === 'monitoring') {
      const label = t('incidents.source.monitoring');
      const ref = incident.source_ref_id;
      if (ref) {
        return `<a class="tag" href="/monitoring?monitor=${encodeURIComponent(ref)}">${escapeHtml(label)} #${escapeHtml(ref)}</a>`;
      }
      return `<span class="tag">${escapeHtml(label)}</span>`;
    }
    return escapeHtml(incident.source);
  }

  async function savePostmortem(incidentId) {
    const tabId = `incident-${incidentId}`;
    const panel = document.querySelector(`#incidents-panels [data-tab="${tabId}"]`);
    const detail = state.incidentDetails.get(incidentId);
    if (!panel || !detail || !detail.incident || detail.readOnly) return;
    const input = panel.querySelector('.incident-postmortem-text');
    if (!input) return;
    const nextPostmortem = (input.value || '').trim();
    try {
      const res = await Api.put(`/api/incidents/${incidentId}/postmortem`, {
        postmortem: nextPostmortem,
        version: detail.incident.version
      });
      const updated = res.incident || res || {};
      if (!updated.meta || typeof updated.meta !== 'object') updated.meta = {};
      updated.meta.postmortem = nextPostmortem;
      detail.incident = updated;
      syncIncident(detail.incident);
      renderIncidentPanel(incidentId);
    } catch (err) {
      showError(err, 'incidents.postmortemSaveFailed');
    }
  }

  function renderSlaState(caseSLA) {
    const values = [];
    if (caseSLA.first_response_due_at) {
      values.push(`<span class="pill ${caseSLA.first_response_late ? 'status-critical' : 'subtle'}">${escapeHtml(t(caseSLA.first_response_late ? 'incidents.sla.firstResponseLate' : 'incidents.sla.firstResponseOk'))}</span>`);
    }
    if (caseSLA.resolve_due_at) {
      values.push(`<span class="pill ${caseSLA.resolve_late ? 'status-critical' : 'subtle'}">${escapeHtml(t(caseSLA.resolve_late ? 'incidents.sla.resolveLate' : 'incidents.sla.resolveOk'))}</span>`);
    }
    if (!values.length) return `<span class="meta-empty">-</span>`;
    return values.join(' ');
  }

  function incidentTags() {
    if (typeof TagDirectory !== 'undefined' && TagDirectory.all) {
      return TagDirectory.all();
    }
    return TAG_FALLBACK;
  }

  function tagLabel(code) {
    const label = (typeof TagDirectory !== 'undefined' && TagDirectory.label) ? TagDirectory.label(code) : null;
    if (label && label !== `docs.tag.${(code || '').toLowerCase()}`) return label;
    const fallback = TAG_FALLBACK.find(t => (t.code || '').toUpperCase() === (code || '').toUpperCase());
    return fallback?.label || code;
  }

  function closeIncidentContext(incidentId) {
    if (!incidentId) return;
    const tab = state.tabs.find(t => t.type === 'incident' && t.incidentId === incidentId);
    if (tab && IncidentsPage.removeTab) {
      IncidentsPage.removeTab(tab.id);
    } else {
      state.incidentDetails.delete(incidentId);
      if (state.pendingStageIncidentId === incidentId) {
        state.pendingStageIncidentId = null;
      }
    }
    state.incidents = state.incidents.filter(i => i.id !== incidentId);
    if (IncidentsPage.renderHome) IncidentsPage.renderHome();
    if (IncidentsPage.renderTableRows) IncidentsPage.renderTableRows();
  }

  IncidentsPage.openIncidentTab = openIncidentTab;
  IncidentsPage.ensureIncidentDetails = ensureIncidentDetails;
  IncidentsPage.renderIncidentPanel = renderIncidentPanel;
  IncidentsPage.renderIncidentInnerTabs = renderIncidentInnerTabs;
  IncidentsPage.renderIncidentInnerContent = renderIncidentInnerContent;
  IncidentsPage.buildTimelineLayout = buildTimelineLayout;
  IncidentsPage.renderIncidentClassification = renderIncidentClassification;
  IncidentsPage.renderIncidentPeople = renderIncidentPeople;
  IncidentsPage.saveIncidentChanges = saveIncidentChanges;
  IncidentsPage.updateIncidentSaveState = updateIncidentSaveState;
  IncidentsPage.isIncidentDirty = isIncidentDirty;
  IncidentsPage.syncIncident = syncIncident;
  IncidentsPage.closeIncidentContext = closeIncidentContext;
  IncidentsPage.updateIncidentTabTitle = updateIncidentTabTitle;
})();
