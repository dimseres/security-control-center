(() => {
  const {
    t,
    escapeHtml,
    formatDate,
    toISODateTime,
    splitISODateTime,
    populateUserSelect,
    getSelectedValues
  } = IncidentsPage;
  const DecisionBlock = IncidentsPage.DecisionBlock;
  const STAGE_SCHEMA = 'incident-stage/v2';
  const BLOCK_ORDER = ['note', 'checklist', 'actions', 'decisions', 'timeline', 'artifacts', 'links', 'table'];
  const DECISION_OUTCOMES = ['closed', 'approved', 'rejected', 'blocked', 'deferred', 'monitor'];
  const CHECKLIST_UNITS = ['minutes', 'hours', 'days', 'weeks', 'months', 'years'];
  const artifactFiles = new Map();

  const STAGE_PRESETS = {
    investigation: {
      blocks: ['table', 'checklist', 'actions', 'artifacts', 'timeline', 'links', 'note'],
      required: ['table'],
      helperKey: 'incidents.stage.presets.investigation.helper',
      descriptionKey: 'incidents.stage.presets.investigation.description',
      blockHints: {
        table: 'incidents.stage.presets.investigation.blocks.table',
        checklist: 'incidents.stage.presets.investigation.blocks.checklist',
        actions: 'incidents.stage.presets.investigation.blocks.actions',
        note: 'incidents.stage.presets.investigation.blocks.note'
      },
      notePlaceholderKey: 'incidents.stage.presets.investigation.notePlaceholder'
    },
    response: {
      blocks: ['actions', 'checklist', 'artifacts', 'links', 'note'],
      required: ['actions'],
      helperKey: 'incidents.stage.presets.response.helper',
      descriptionKey: 'incidents.stage.presets.response.description',
      blockHints: {
        actions: 'incidents.stage.presets.response.blocks.actions',
        checklist: 'incidents.stage.presets.response.blocks.checklist',
        artifacts: 'incidents.stage.presets.response.blocks.artifacts',
        note: 'incidents.stage.presets.response.blocks.note'
      },
      notePlaceholderKey: 'incidents.stage.presets.response.notePlaceholder'
    },
    closure: {
      blocks: ['decisions'],
      required: ['decisions'],
      helperKey: 'incidents.stage.presets.closure.helper',
      descriptionKey: 'incidents.stage.presets.closure.description',
      blockHints: {
        decisions: 'incidents.stage.presets.closure.blocks.decisions',
      },
      notePlaceholderKey: 'incidents.stage.presets.closure.notePlaceholder'
    },
    decision: {
      blocks: ['decisions', 'note', 'links', 'timeline'],
      required: ['decisions'],
      helperKey: 'incidents.stage.presets.decision.helper',
      descriptionKey: 'incidents.stage.presets.decision.description',
      blockHints: {
        decisions: 'incidents.stage.presets.decision.blocks.decisions',
        note: 'incidents.stage.presets.decision.blocks.note'
      },
      notePlaceholderKey: 'incidents.stage.presets.decision.notePlaceholder'
    }
  };

  function splitDateTime(val) {
    if (typeof splitISODateTime === 'function') {
      return splitISODateTime(val || '');
    }
    return { date: '', time: '' };
  }

  function normalizeTimeValue(value) {
    const raw = (value || '').trim();
    if (!raw) return '';
    const parts = raw.split(':').map(p => p.trim());
    const hour = parseInt(parts[0] || '0', 10);
    const minute = parseInt(parts[1] || '0', 10);
    if ([hour, minute].some(Number.isNaN)) return raw;
    if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return raw;
    const pad = (num) => `${num}`.padStart(2, '0');
    return `${pad(hour)}:${pad(minute)}`;
  }

  function toISODateValue(value) {
    if (!value) return '';
    if (typeof AppTime !== 'undefined' && AppTime.toISODate) {
      const parsed = AppTime.toISODate(value);
      if (parsed) return parsed;
    }
    return value;
  }

  function mergeDateTime(date, time) {
    if (typeof toISODateTime === 'function') {
      return toISODateTime(date, time);
    }
    if (!date && !time) return '';
    return `${date}T${time}`;
  }

  function renderDateTimeInput(value, onChange) {
    const wrap = document.createElement('div');
    wrap.className = 'datetime-field';
    const dateInput = document.createElement('input');
    dateInput.type = 'date';
    dateInput.className = 'input date-input';
    dateInput.lang = (typeof BerkutI18n !== 'undefined' && BerkutI18n.currentLang && BerkutI18n.currentLang() === 'en') ? 'en' : 'ru';
    const timeInput = document.createElement('input');
    timeInput.type = 'time';
    timeInput.step = '60';
    timeInput.className = 'input time-input';
    const { date, time } = splitDateTime(value);
    dateInput.value = toISODateValue(date);
    timeInput.value = time;
    const sync = () => {
      const combined = mergeDateTime(dateInput.value, timeInput.value);
      onChange(combined);
    };
    const normalizeTime = () => {
      const normalized = normalizeTimeValue(timeInput.value);
      if (normalized !== timeInput.value) {
        timeInput.value = normalized;
      }
      sync();
    };
    dateInput.addEventListener('change', sync);
    dateInput.addEventListener('input', sync);
    timeInput.addEventListener('change', sync);
    timeInput.addEventListener('input', sync);
    timeInput.addEventListener('blur', normalizeTime);
    wrap.appendChild(dateInput);
    wrap.appendChild(timeInput);
    return wrap;
  }

  function renderUserSelect(selected, onChange) {
    const select = document.createElement('select');
    select.className = 'select';
    const value = selected ? [selected] : [];
    if (populateUserSelect) {
      populateUserSelect(select, value);
    }
    if (IncidentsPage.enforceSingleSelect) {
      IncidentsPage.enforceSingleSelect(select);
    }
    const sync = () => {
      if (typeof getSelectedValues === 'function') {
        const val = getSelectedValues(select)[0] || '';
        onChange(val);
      } else {
        onChange(select.value || '');
      }
    };
    select.addEventListener('change', sync);
    select.addEventListener('selectionrefresh', sync);
    sync();
    return select;
  }

  function uid(prefix = 'blk') {
    return `${prefix}-${Math.random().toString(36).slice(2, 8)}-${Date.now()}`;
  }

  function getPreset(stageType) {
    return STAGE_PRESETS[stageType] || null;
  }

  function presetBlocks(stageType) {
    const preset = getPreset(stageType);
    return preset ? preset.blocks.slice() : null;
  }

  function createBlockTemplate(type) {
    switch (type) {
      case 'checklist':
        return {
          id: uid('checklist'),
          type: 'checklist',
          items: [{
            id: uid('item'),
            text: '',
            status: 'not_done',
            owner: '',
            due_value: '',
            due_unit: 'hours',
            status_changed_at: ''
          }]
        };
      case 'actions':
        return {
          id: uid('actions'),
          type: 'actions',
          items: [{ id: uid('item'), action: '', owner: '', at: '', result: '' }]
        };
      case 'decisions':
        if (DecisionBlock?.createBlockTemplate) return DecisionBlock.createBlockTemplate();
        return {
          id: uid('decision'),
          type: 'decisions',
          items: [{ id: uid('item'), decision: '', rationale: '', owner: '', date: '' }]
        };
      case 'timeline':
        return {
          id: uid('timeline'),
          type: 'timeline',
          items: [{ id: uid('item'), at: '', event_type: '', message: '' }]
        };
      case 'artifacts':
        return {
          id: uid('artifact'),
          type: 'artifacts',
          items: [{ id: uid('item'), title: '', reference: '', note: '', files: [] }]
        };
      case 'links':
        return {
          id: uid('link'),
          type: 'links',
          items: [{ id: uid('item'), link_type: 'doc', reference: '', comment: '' }]
        };
      case 'table':
        return {
          id: uid('table'),
          type: 'table',
          items: [{ id: uid('item'), indicator: '', indicator_type: '', context: '' }]
        };
      default:
        return { id: uid('note'), type: 'note', text: '' };
    }
  }

  function normalizeBlock(block, stageType) {
    if (!block || !block.type || !BLOCK_ORDER.includes(block.type)) {
      return createBlockTemplate('note');
    }
    if (!block.id) block.id = uid(block.type);
    if (block.type === 'note') {
      block.text = (block.text || '').toString();
      return block;
    }
    if (block.type === 'decisions' && DecisionBlock?.normalizeBlock) {
      return DecisionBlock.normalizeBlock(block);
    }
    if (!Array.isArray(block.items)) {
      block.items = [];
    }
    if (!block.items.length) {
      block.items.push(createBlockTemplate(block.type).items[0]);
    }
    if (block.type === 'checklist') {
      block.items = block.items.map(item => ({
        id: item.id || uid('item'),
        text: item.text || '',
        owner: item.owner || '',
        status: (item.status || item.state || 'not_done') === 'done' ? 'done' : 'not_done',
        due_value: item.due_value || item.due || '',
        due_unit: CHECKLIST_UNITS.includes(item.due_unit) ? item.due_unit : 'hours',
        status_changed_at: item.status_changed_at || item.changed_at || ''
      }));
      return block;
    }
    if (block.type === 'artifacts') {
      block.items = block.items.map(item => ({
        id: item.id || uid('item'),
        title: item.title || '',
        reference: item.reference || '',
        note: item.note || '',
        files: Array.isArray(item.files) ? item.files : []
      }));
      return block;
    }
    block.items = block.items.map(item => ({ id: item.id || uid('item'), ...item }));
    return block;
  }

  function normalizeBlocks(blocks, stageType) {
    if (!Array.isArray(blocks) || !blocks.length) {
      const preset = presetBlocks(stageType);
      if (preset && preset.length) {
        return preset.map(type => createBlockTemplate(type));
      }
      return [createBlockTemplate('note')];
    }
    return blocks.map(block => normalizeBlock(block, stageType));
  }

  function fallbackBlocks(raw) {
    const text = (raw || '').toString().trim();
    if (!text) return [createBlockTemplate('note')];
    const note = createBlockTemplate('note');
    note.text = text;
    return [note];
  }

  function parseContent(raw) {
    if (!raw) {
      const blocks = normalizeBlocks(null, 'custom');
      return { stageType: 'custom', blocks, serialized: serializePayload({ stageType: 'custom', blocks }) };
    }
    try {
      const parsed = JSON.parse(raw);
      const stageType = parsed.stageType || parsed.type || 'custom';
      const blocks = normalizeBlocks(parsed.blocks || fallbackBlocks(parsed.text || ''), stageType);
      return { stageType, blocks, serialized: serializePayload({ stageType, blocks }) };
    } catch (err) {
      const blocks = fallbackBlocks(raw);
      return { stageType: 'custom', blocks, serialized: serializePayload({ stageType: 'custom', blocks }) };
    }
  }

  function serializePayload(payload) {
    const clean = {
      schema: STAGE_SCHEMA,
      stageType: payload.stageType || 'custom',
      blocks: normalizeBlocks(payload.blocks || [], payload.stageType)
    };
    return JSON.stringify(clean);
  }

  function serializeContent(stage) {
    return serializePayload({
      stageType: stage?.stageType || 'custom',
      blocks: stage?.blocks || []
    });
  }

  function createBlocksFromSelection(selected, stageType) {
    const normalized = Array.isArray(selected) ? selected : [];
    const unique = [];
    normalized.forEach(type => {
      if (BLOCK_ORDER.includes(type) && !unique.includes(type)) unique.push(type);
    });
    const fallback = presetBlocks(stageType) || ['note'];
    const chosen = unique.length ? unique : fallback;
    return chosen.map(type => createBlockTemplate(type));
  }

  function blockTitle(type) {
    return t(`incidents.stage.blocks.${type}`) || type;
  }

  function blockHint(stageType, type) {
    const preset = getPreset(stageType);
    const key = preset?.blockHints?.[type];
    if (!key) return '';
    const text = t(key);
    return text && !text.startsWith('incidents.') ? text : '';
  }

  function notePlaceholder(stageType) {
    const preset = getPreset(stageType);
    const key = preset?.notePlaceholderKey;
    const hint = key ? t(key) : '';
    const fallback = t('incidents.stage.blocks.notePlaceholder');
    if (hint && !hint.startsWith('incidents.')) return hint;
    return fallback && !fallback.startsWith('incidents.') ? fallback : '';
  }

  function stageHelper(stageType) {
    const preset = getPreset(stageType);
    const key = preset?.helperKey;
    const text = key ? t(key) : '';
    return text && !text.startsWith('incidents.') ? text : '';
  }

  function stageDescription(stageType) {
    const preset = getPreset(stageType);
    const key = preset?.descriptionKey;
    const text = key ? t(key) : '';
    return text && !text.startsWith('incidents.') ? text : '';
  }

  function ensureStageDefaults(stage) {
    if (!stage) return stage;
    stage.blocks = normalizeBlocks(stage.blocks, stage.stageType);
    return stage;
  }

  function notifyChange(ctx) {
    if (typeof ctx?.onChange === 'function') ctx.onChange();
  }

  function renderBlocks(stage, container, ctx = {}) {
    const context = typeof ctx === 'function' ? { onChange: ctx } : (ctx || {});
    if (!container || !stage) return;
    ensureStageDefaults(stage);
    const rerender = () => renderBlocks(stage, container, context);
    container.innerHTML = '';
    stage.blocks.forEach(block => {
      const card = document.createElement('div');
      card.className = 'stage-block';
      card.dataset.type = block.type;
      const head = document.createElement('div');
      head.className = 'stage-block-header';
      const titleWrap = document.createElement('div');
      titleWrap.className = 'stage-block-title-wrap';
      const title = document.createElement('div');
      title.className = 'stage-block-title';
      title.textContent = blockTitle(block.type);
      titleWrap.appendChild(title);
      const hintText = blockHint(stage.stageType, block.type);
      if (hintText) {
        const hintEl = document.createElement('div');
        hintEl.className = 'stage-block-hint';
        hintEl.textContent = hintText;
        titleWrap.appendChild(hintEl);
      }
      head.appendChild(titleWrap);
      card.appendChild(head);
      const body = document.createElement('div');
      body.className = 'stage-block-body';
      renderBlockBody(block, body, { ...context, rerender, stage });
      card.appendChild(body);
      container.appendChild(card);
    });
  }

  function renderBlockBody(block, target, ctx) {
    switch (block.type) {
      case 'note':
        renderNoteBlock(block, target, ctx);
        return;
      case 'checklist':
        renderChecklistBlock(block, target, ctx);
        return;
      case 'actions':
        renderActionsBlock(block, target, ctx);
        return;
      case 'decisions':
        if (DecisionBlock?.render) {
          DecisionBlock.render(block, target, ctx);
          return;
        }
        renderListBlock(block, target, ctx, {
          columns: [
            { key: 'decision', label: t('incidents.stage.blocks.decisions.decision'), type: 'text', placeholder: t('incidents.stage.blocks.decisions.placeholder') },
            { key: 'rationale', label: t('incidents.stage.blocks.decisions.rationale'), type: 'text', placeholder: t('incidents.stage.blocks.decisions.rationalePlaceholder'), grow: 1.5 },
            { key: 'owner', label: t('incidents.stage.blocks.decisions.by'), type: 'text', placeholder: t('incidents.stage.blocks.ownerPlaceholder') },
            { key: 'date', label: t('incidents.stage.blocks.decisions.date'), type: 'date', placeholder: t('incidents.stage.blocks.datePlaceholder') },
          ],
          addLabel: t('incidents.stage.blocks.decisions.add')
        });
        return;
      case 'timeline':
        renderTimelineBlock(block, target, ctx);
        return;
      case 'artifacts':
        renderArtifactsBlock(block, target, ctx);
        return;
      case 'links':
        renderLinksBlock(block, target, ctx);
        return;
      case 'table':
        renderListBlock(block, target, ctx, {
          columns: [
            { key: 'indicator', label: t('incidents.stage.blocks.table.indicator'), type: 'text', placeholder: t('incidents.stage.blocks.table.indicatorPlaceholder'), grow: 1.2 },
            { key: 'indicator_type', label: t('incidents.stage.blocks.table.indicatorType'), type: 'text', placeholder: t('incidents.stage.blocks.table.indicatorTypePlaceholder') },
            { key: 'context', label: t('incidents.stage.blocks.table.context'), type: 'text', placeholder: t('incidents.stage.blocks.table.contextPlaceholder'), grow: 1.4 },
          ],
          addLabel: t('incidents.stage.blocks.table.add')
        });
        return;
      default:
        renderNoteBlock(block, target, ctx);
    }
  }

  function renderNoteBlock(block, target, ctx) {
    const textarea = document.createElement('textarea');
    textarea.className = 'textarea';
    textarea.placeholder = notePlaceholder(ctx.stage?.stageType);
    textarea.value = block.text || '';
    textarea.addEventListener('input', (e) => {
      block.text = e.target.value;
      notifyChange(ctx.onChange);
    });
    target.appendChild(textarea);
  }

  function renderChecklistBlock(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row checklist-row';
      const statusWrap = document.createElement('div');
      statusWrap.className = 'checklist-status';
      const doneBtn = document.createElement('button');
      doneBtn.type = 'button';
      doneBtn.className = `btn pill-btn ${item.status === 'done' ? 'pill-active' : ''}`;
      doneBtn.textContent = t('incidents.stage.blocks.checklist.done');
      doneBtn.addEventListener('click', () => {
        item.status = 'done';
        item.status_changed_at = new Date().toISOString();
        notifyChange(ctx);
        ctx.rerender();
      });
      const notDoneBtn = document.createElement('button');
      notDoneBtn.type = 'button';
      notDoneBtn.className = `btn pill-btn danger ${item.status !== 'done' ? 'pill-active' : ''}`;
      notDoneBtn.textContent = t('incidents.stage.blocks.checklist.notDone');
      notDoneBtn.addEventListener('click', () => {
        item.status = 'not_done';
        item.status_changed_at = new Date().toISOString();
        notifyChange(ctx);
        ctx.rerender();
      });
      statusWrap.appendChild(doneBtn);
      statusWrap.appendChild(notDoneBtn);
      row.appendChild(statusWrap);
      const content = document.createElement('div');
      content.className = 'checklist-content';
      const titleField = document.createElement('label');
      titleField.className = 'stage-field';
      titleField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.checklist.item')}</span>`;
      const titleInput = document.createElement('input');
      titleInput.className = 'input';
      titleInput.placeholder = t('incidents.stage.blocks.checklist.placeholder');
      titleInput.value = item.text || '';
      titleInput.addEventListener('input', (e) => {
        item.text = e.target.value;
        notifyChange(ctx);
      });
      titleField.appendChild(titleInput);
      content.appendChild(titleField);

      const metaRow = document.createElement('div');
      metaRow.className = 'checklist-meta';
      const ownerField = document.createElement('label');
      ownerField.className = 'stage-field';
      const ownerCaption = document.createElement('span');
      ownerCaption.className = 'stage-field-label';
      ownerCaption.textContent = t('incidents.stage.blocks.checklist.owner');
      ownerField.appendChild(ownerCaption);
      const ownerSelect = renderUserSelect(item.owner || '', (val) => {
        item.owner = val;
        notifyChange(ctx);
      });
      ownerField.appendChild(ownerSelect);
      metaRow.appendChild(ownerField);

      const dueField = document.createElement('label');
      dueField.className = 'stage-field due-field';
      dueField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.checklist.due')}</span>`;
      const dueWrap = document.createElement('div');
      dueWrap.className = 'due-inputs';
      const dueValue = document.createElement('input');
      dueValue.type = 'number';
      dueValue.min = '0';
      dueValue.className = 'input';
      dueValue.value = item.due_value || '';
      dueValue.placeholder = '0';
      dueValue.addEventListener('input', (e) => {
        item.due_value = e.target.value;
        notifyChange(ctx);
      });
      const dueUnit = document.createElement('select');
      dueUnit.className = 'select';
      CHECKLIST_UNITS.forEach(unit => {
        const opt = document.createElement('option');
        opt.value = unit;
        opt.textContent = t(`incidents.stage.blocks.checklist.unit.${unit}`);
        dueUnit.appendChild(opt);
      });
      dueUnit.value = CHECKLIST_UNITS.includes(item.due_unit) ? item.due_unit : 'hours';
      dueUnit.addEventListener('change', (e) => {
        item.due_unit = e.target.value;
        notifyChange(ctx);
      });
      dueWrap.appendChild(dueValue);
      dueWrap.appendChild(dueUnit);
      dueField.appendChild(dueWrap);
      metaRow.appendChild(dueField);

      if (item.status_changed_at) {
        const stamp = document.createElement('div');
        stamp.className = 'checklist-stamp';
        stamp.textContent = `${t('incidents.stage.blocks.checklist.changedAt')}: ${formatDate(item.status_changed_at)}`;
        metaRow.appendChild(stamp);
      }

      content.appendChild(metaRow);
      row.appendChild(content);
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = '-';
      remove.title = t('common.delete');
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate('checklist').items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.checklist.addItem');
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate('checklist').items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function renderActionsBlock(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row';
      const actionField = document.createElement('label');
      actionField.className = 'stage-field';
      actionField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.actions.what')}</span>`;
      const actionInput = document.createElement('input');
      actionInput.className = 'input';
      actionInput.placeholder = t('incidents.stage.blocks.actions.placeholder');
      actionInput.value = item.action || '';
      actionInput.addEventListener('input', (e) => {
        item.action = e.target.value;
        notifyChange(ctx);
      });
      actionField.appendChild(actionInput);
      row.appendChild(actionField);

      const ownerField = document.createElement('label');
      ownerField.className = 'stage-field';
      ownerField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.actions.owner')}</span>`;
      const ownerSelect = renderUserSelect(item.owner || '', (val) => {
        item.owner = val;
        notifyChange(ctx);
      });
      ownerField.appendChild(ownerSelect);
      row.appendChild(ownerField);

      const whenField = document.createElement('label');
      whenField.className = 'stage-field';
      whenField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.actions.when')}</span>`;
      const dateInput = renderDateTimeInput(item.at || '', (val) => {
        item.at = val;
        notifyChange(ctx);
      });
      whenField.appendChild(dateInput);
      row.appendChild(whenField);

      const resultField = document.createElement('label');
      resultField.className = 'stage-field';
      resultField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.actions.result')}</span>`;
      const resultInput = document.createElement('textarea');
      resultInput.className = 'textarea';
      resultInput.placeholder = t('incidents.stage.blocks.actions.resultPlaceholder');
      resultInput.value = item.result || '';
      resultInput.addEventListener('input', (e) => {
        item.result = e.target.value;
        notifyChange(ctx);
      });
      resultField.appendChild(resultInput);
      row.appendChild(resultField);

      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = '-';
      remove.title = t('common.delete');
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate('actions').items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.actions.add');
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate('actions').items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function renderTimelineBlock(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row';
      const whenField = document.createElement('label');
      whenField.className = 'stage-field';
      whenField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.timeline.when')}</span>`;
      const dateInput = renderDateTimeInput(item.at || '', (val) => {
        item.at = val;
        notifyChange(ctx);
      });
      whenField.appendChild(dateInput);
      row.appendChild(whenField);

      const typeField = document.createElement('label');
      typeField.className = 'stage-field';
      typeField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.timeline.type')}</span>`;
      const typeInput = document.createElement('input');
      typeInput.className = 'input';
      typeInput.placeholder = t('incidents.stage.blocks.timeline.typePlaceholder');
      typeInput.value = item.event_type || '';
      typeInput.addEventListener('input', (e) => {
        item.event_type = e.target.value;
        notifyChange(ctx);
      });
      typeField.appendChild(typeInput);
      row.appendChild(typeField);

      const msgField = document.createElement('label');
      msgField.className = 'stage-field';
      msgField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.timeline.message')}</span>`;
      const msgInput = document.createElement('textarea');
      msgInput.className = 'textarea';
      msgInput.placeholder = t('incidents.stage.blocks.timeline.messagePlaceholder');
      msgInput.value = item.message || '';
      msgInput.addEventListener('input', (e) => {
        item.message = e.target.value;
        notifyChange(ctx);
      });
      msgField.appendChild(msgInput);
      row.appendChild(msgField);

      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = '-';
      remove.title = t('common.delete');
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate('timeline').items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.timeline.add');
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate('timeline').items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function renderLinksBlock(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row';
      const typeField = document.createElement('label');
      typeField.className = 'stage-field';
      typeField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.links.type')}</span>`;
      const typeSelect = document.createElement('select');
      typeSelect.className = 'select';
      const typeOptions = [
        { value: 'doc', label: t('incidents.links.type.doc') },
        { value: 'incident', label: t('incidents.links.type.incident') },
        { value: 'report', label: t('incidents.links.type.report') },
        { value: 'task', label: t('incidents.links.type.task') },
        { value: 'other', label: t('incidents.links.type.other') },
      ];
      typeOptions.forEach(opt => {
        const o = document.createElement('option');
        o.value = opt.value;
        o.textContent = opt.label;
        typeSelect.appendChild(o);
      });
      typeSelect.value = item.link_type || 'doc';
      typeSelect.addEventListener('change', () => {
        item.link_type = typeSelect.value;
        item.reference = '';
        notifyChange(ctx);
        ctx.rerender();
      });
      typeField.appendChild(typeSelect);
      row.appendChild(typeField);

      const refField = document.createElement('label');
      refField.className = 'stage-field';
      refField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.links.reference')}</span>`;
      const refSelect = document.createElement('select');
      refSelect.className = 'select';
      const commentField = document.createElement('label');
      commentField.className = 'stage-field';
      commentField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.links.comment')}</span>`;
      const commentInput = document.createElement('input');
      commentInput.className = 'input';
      commentInput.placeholder = t('incidents.stage.blocks.notesPlaceholder');
      commentInput.value = item.comment || '';
      commentInput.addEventListener('input', (e) => {
        item.comment = e.target.value;
        notifyChange(ctx);
      });
      commentField.appendChild(commentInput);

      const setOptions = (options) => {
        refSelect.innerHTML = '';
        if (!options.length) {
          const emptyOpt = document.createElement('option');
          emptyOpt.value = '';
          emptyOpt.textContent = t('incidents.links.empty');
          refSelect.appendChild(emptyOpt);
        } else {
          const placeholder = document.createElement('option');
          placeholder.value = '';
          placeholder.textContent = t('incidents.stage.blocks.links.referencePlaceholder');
          refSelect.appendChild(placeholder);
        }
        options.forEach(opt => {
          const o = document.createElement('option');
          o.value = opt.value;
          o.textContent = opt.label;
          refSelect.appendChild(o);
        });
        refSelect.value = item.reference || '';
      };

      const fillOptions = () => {
        const type = item.link_type || 'doc';
        if (type === 'other') {
          refSelect.disabled = true;
          setOptions([]);
          item.reference = '';
          commentInput.required = true;
          return;
        }
        commentInput.required = false;
        refSelect.disabled = false;
        const loading = [{ value: '', label: t('common.loading') || '...' }];
        setOptions(loading);
        if (typeof IncidentsPage.ensureLinkOptions === 'function' && ctx.incidentId) {
          IncidentsPage.ensureLinkOptions(ctx.incidentId).then(opts => {
            let values = [];
            if (type === 'doc') values = (opts.docs || []).map(d => ({ value: d.id, label: IncidentsPage.linkOptionLabel ? IncidentsPage.linkOptionLabel('doc', d) : d.title || d.id }));
            if (type === 'report') values = (opts.docs || []).filter(d => (d.type || '').toLowerCase() === 'report').map(d => ({ value: d.id, label: IncidentsPage.linkOptionLabel ? IncidentsPage.linkOptionLabel('report', d) : d.title || d.id }));
            if (type === 'incident') values = (opts.incidents || []).map(i => ({ value: i.id, label: IncidentsPage.linkOptionLabel ? IncidentsPage.linkOptionLabel('incident', i) : `#${i.id} ${i.title || ''}`.trim() }));
            if (type === 'task') values = (opts.tasks || []).map(ti => ({ value: ti.id, label: IncidentsPage.linkOptionLabel ? IncidentsPage.linkOptionLabel('task', ti) : ti.title || ti.id }));
            setOptions(values);
          });
        } else {
          setOptions([]);
        }
      };
      refSelect.addEventListener('change', () => {
        item.reference = refSelect.value;
        notifyChange(ctx);
      });
      refField.appendChild(refSelect);
      row.appendChild(refField);
      row.appendChild(commentField);

      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = '-';
      remove.title = t('common.delete');
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate('links').items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);
      fillOptions();
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.links.add');
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate('links').items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function getArtifactFileKey(incidentId, artifactId) {
    return `${incidentId || 'unknown'}:${artifactId}`;
  }

  async function loadArtifactFiles(incidentId, artifactId) {
    const key = getArtifactFileKey(incidentId, artifactId);
    const cached = artifactFiles.get(key);
    if (cached && cached.loading) return cached.promise;
    if (cached && cached.items) return cached.items;
    const state = { loading: true, items: [] };
    artifactFiles.set(key, state);
    const promise = (async () => {
      if (!incidentId || !artifactId) return [];
      try {
        const res = await Api.get(`/api/incidents/${incidentId}/artifacts/${artifactId}/files`);
        state.items = res.items || [];
        return state.items;
      } catch (err) {
        state.items = [];
        return [];
      } finally {
        state.loading = false;
      }
    })();
    state.promise = promise;
    return promise;
  }

  async function uploadArtifactFile(incidentId, artifactId, file, onSuccess, onError) {
    if (!incidentId || !artifactId || !file) return;
    try {
      const fd = new FormData();
      fd.append('file', file);
      await Api.upload(`/api/incidents/${incidentId}/artifacts/${artifactId}/files`, fd);
      if (onSuccess) onSuccess();
    } catch (err) {
      if (onError) onError(err);
    }
  }

  async function deleteArtifactFile(incidentId, artifactId, fileId, onSuccess, onError) {
    if (!incidentId || !artifactId || !fileId) return;
    try {
      await Api.del(`/api/incidents/${incidentId}/artifacts/${artifactId}/files/${fileId}`);
      if (onSuccess) onSuccess();
    } catch (err) {
      if (onError) onError(err);
    }
  }

  function renderArtifactFilesList(container, files) {
    container.innerHTML = '';
    if (!files || !files.length) {
      const empty = document.createElement('div');
      empty.className = 'meta-empty';
      empty.textContent = t('incidents.stage.blocks.artifacts.filesEmpty');
      container.appendChild(empty);
      return;
    }
    files.forEach(file => {
      const row = document.createElement('div');
      row.className = 'artifact-file-row';
      const title = document.createElement('div');
      title.className = 'artifact-file-name';
      title.textContent = file.filename || file.name || '';
      row.appendChild(title);
      const meta = document.createElement('div');
      meta.className = 'artifact-file-meta';
      const time = file.uploaded_at ? formatDate(file.uploaded_at) : '';
      meta.textContent = `${file.uploaded_by_name || ''} ${time ? `• ${time}` : ''}`;
      row.appendChild(meta);
      const actions = document.createElement('div');
      actions.className = 'artifact-file-actions';
      const download = document.createElement('button');
      download.type = 'button';
      download.className = 'btn ghost icon-btn';
      download.textContent = '↓';
      download.title = t('incidents.stage.blocks.artifacts.download');
      download.dataset.allowRead = '1';
      download.addEventListener('click', () => {
        const incidentId = container.dataset.incidentId;
        const artifactId = file.artifact_id || file.artifactId || container.dataset.artifactId || '';
        if (!incidentId || !artifactId || !file.id) return;
        window.open(`/api/incidents/${incidentId}/artifacts/${artifactId}/files/${file.id}/download`, '_blank');
      });
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn';
      remove.textContent = '-';
      remove.title = t('incidents.stage.blocks.artifacts.removeFile');
      remove.addEventListener('click', async () => {
        const incidentId = container.dataset.incidentId;
        const artifactId = file.artifact_id || file.artifactId || container.dataset.artifactId || '';
        if (!incidentId || !artifactId || !file.id) return;
        if (IncidentsPage.confirmAction) {
          const ok = await IncidentsPage.confirmAction({
            title: t('common.confirm'),
            message: t('incidents.stage.blocks.artifacts.removeFileConfirm'),
            confirmText: t('common.delete'),
            cancelText: t('common.cancel')
          });
          if (!ok) return;
        }
        deleteArtifactFile(incidentId, artifactId, file.id, () => {
          loadArtifactFiles(incidentId, artifactId).then(items => {
            renderArtifactFilesList(container, items);
          });
        });
      });
      actions.appendChild(download);
      actions.appendChild(remove);
      row.appendChild(actions);
      container.appendChild(row);
    });
  }

  function renderArtifactsBlock(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row artifact-row';
      const fields = document.createElement('div');
      fields.className = 'artifact-fields';

      const titleField = document.createElement('label');
      titleField.className = 'stage-field';
      titleField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.artifacts.title')}</span>`;
      const titleInput = document.createElement('input');
      titleInput.className = 'input';
      titleInput.placeholder = t('incidents.stage.blocks.artifacts.placeholder');
      titleInput.value = item.title || '';
      titleInput.addEventListener('input', (e) => {
        item.title = e.target.value;
        notifyChange(ctx);
      });
      titleField.appendChild(titleInput);
      fields.appendChild(titleField);

      const refField = document.createElement('label');
      refField.className = 'stage-field';
      refField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.artifacts.reference')}</span>`;
      const refInput = document.createElement('input');
      refInput.className = 'input';
      refInput.placeholder = t('incidents.stage.blocks.artifacts.referencePlaceholder');
      refInput.value = item.reference || '';
      refInput.addEventListener('input', (e) => {
        item.reference = e.target.value;
        notifyChange(ctx);
      });
      refField.appendChild(refInput);
      fields.appendChild(refField);

      const noteField = document.createElement('label');
      noteField.className = 'stage-field';
      noteField.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.artifacts.note')}</span>`;
      const noteInput = document.createElement('textarea');
      noteInput.className = 'textarea';
      noteInput.placeholder = t('incidents.stage.blocks.notesPlaceholder');
      noteInput.value = item.note || '';
      noteInput.addEventListener('input', (e) => {
        item.note = e.target.value;
        notifyChange(ctx);
      });
      noteField.appendChild(noteInput);
      fields.appendChild(noteField);

      row.appendChild(fields);

      const filesWrap = document.createElement('div');
      filesWrap.className = 'artifact-files';
      filesWrap.dataset.incidentId = ctx.incidentId || '';
      const filesHeader = document.createElement('div');
      filesHeader.className = 'artifact-files-header';
      const filesTitle = document.createElement('span');
      filesTitle.textContent = t('incidents.stage.blocks.artifacts.files');
      filesHeader.appendChild(filesTitle);
      const uploadInput = document.createElement('input');
      uploadInput.type = 'file';
      uploadInput.multiple = true;
      uploadInput.hidden = true;
      const uploadBtn = document.createElement('button');
      uploadBtn.type = 'button';
      uploadBtn.className = 'btn ghost';
      uploadBtn.textContent = t('incidents.stage.blocks.artifacts.attach');
      if (!ctx.incidentId) uploadBtn.disabled = true;
      uploadBtn.addEventListener('click', () => uploadInput.click());
      uploadInput.addEventListener('change', async () => {
        const files = Array.from(uploadInput.files || []);
        for (const f of files) {
          await uploadArtifactFile(ctx.incidentId, item.id, f, () => {}, (err) => {
            IncidentsPage.showError ? IncidentsPage.showError(err, 'incidents.stage.blocks.artifacts.uploadFailed') : null;
          });
        }
        uploadInput.value = '';
        const refreshed = await loadArtifactFiles(ctx.incidentId, item.id);
        renderArtifactFilesList(listContainer, refreshed);
      });
      filesHeader.appendChild(uploadBtn);
      filesWrap.appendChild(filesHeader);

      const listContainer = document.createElement('div');
      listContainer.className = 'artifact-files-list';
      listContainer.dataset.incidentId = ctx.incidentId || '';
      listContainer.dataset.artifactId = item.id || '';
      filesWrap.appendChild(listContainer);
      row.appendChild(filesWrap);

      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = '-';
      remove.title = t('common.delete');
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate('artifacts').items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);

      loadArtifactFiles(ctx.incidentId, item.id).then(files => {
        renderArtifactFilesList(listContainer, files || []);
      });
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.artifacts.add');
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate('artifacts').items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function renderListBlock(block, target, ctx, opts) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'stage-rows';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'stage-row';
      opts.columns.forEach(col => {
        const field = document.createElement('label');
        field.className = 'stage-field';
        if (col.grow) field.style.flex = col.grow;
        const caption = document.createElement('span');
        caption.className = 'stage-field-label';
        caption.textContent = col.label;
        field.appendChild(caption);
        let input;
        if (col.type === 'select') {
          input = document.createElement('select');
          input.className = 'select';
          (col.options || []).forEach(opt => {
            const option = document.createElement('option');
            option.value = opt.value;
            option.textContent = opt.label;
            input.appendChild(option);
          });
          input.value = item[col.key] || (col.options?.[0]?.value || '');
        } else if (col.type === 'textarea') {
          input = document.createElement('textarea');
          input.className = 'textarea';
          input.value = item[col.key] || '';
        } else {
          input = document.createElement('input');
          input.className = col.type === 'date' ? 'input date-input' : 'input';
          input.type = col.type === 'date' ? 'date' : 'text';
          if (col.type === 'date') input.lang = 'ru';
          input.value = col.type === 'date' ? toISODateValue(item[col.key] || '') : (item[col.key] || '');
        }
        if (col.placeholder) input.placeholder = col.placeholder;
        input.addEventListener('input', (e) => {
          item[col.key] = e.target.value;
          notifyChange(ctx);
        });
        if (col.type === 'select') {
          input.addEventListener('change', (e) => {
            item[col.key] = e.target.value;
            notifyChange(ctx);
          });
        }
        field.appendChild(input);
        row.appendChild(field);
      });
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn stage-row-remove';
      remove.textContent = 'x';
      const removeLabel = t('common.delete');
      remove.title = removeLabel && !removeLabel.startsWith('common.') ? removeLabel : 'Remove';
      remove.setAttribute('aria-label', remove.title);
      remove.addEventListener('click', () => {
        block.items = block.items.filter(it => it.id !== item.id);
        if (!block.items.length) {
          block.items.push(createBlockTemplate(block.type).items[0]);
        }
        notifyChange(ctx);
        ctx.rerender();
      });
      row.appendChild(remove);
      wrapper.appendChild(row);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = opts.addLabel || t('common.add') || 'Add';
    add.addEventListener('click', () => {
      block.items.push(createBlockTemplate(block.type).items[0]);
      notifyChange(ctx);
      ctx.rerender();
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  IncidentsPage.StageBlocks = {
    STAGE_SCHEMA,
    BLOCK_ORDER,
    STAGE_PRESETS,
    createBlockTemplate,
    createBlocksFromSelection,
    parseContent,
    serializeContent,
    renderBlocks,
    blockTitle,
    blockHint,
    stageHelper,
    stageDescription,
    ensureStageDefaults
  };
})();
