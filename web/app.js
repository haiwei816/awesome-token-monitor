// Universal AI Agent Token & Model Monitor Client (Multi-platform & Daily Ledger)
function getLocalDateString() {
  const d = new Date();
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}
const todayDateStr = getLocalDateString();

let currentAgent = 'all';
let currentModel = 'all';
let currentDate = todayDateStr; // 打开的时候先只显示今天的 (默认今日)
let currentGranularity = 'day';
let currentSort = 'total_tokens';
let searchQuery = '';
let currentPage = 0;
const pageSize = 20;

let trendChartInstance = null;
let stepGrowthChartInstance = null;
let autoRefreshTimer = null;
let latestSummaryData = null;

// Agent Info Configuration
const AGENT_CONFIG = {
  codex:        { name: 'Codex',         icon: '🧠', badgeClass: 'badge-codex' },
  claude:       { name: 'Claude Code',   icon: '🟣', badgeClass: 'badge-claude' },
  pi:           { name: 'Pi Agent',      icon: '🥧', badgeClass: 'badge-pi' },
  qoder:        { name: 'Qoder',         icon: '⚡', badgeClass: 'badge-qoder' },
  antigravity:  { name: 'Antigravity',   icon: '🛰️', badgeClass: 'badge-antigravity' },
  workbuddy:    { name: 'WorkBuddy',     icon: '💼', badgeClass: 'badge-workbuddy' },
  zcode:        { name: 'ZCode',         icon: '🔷', badgeClass: 'badge-zcode' }
};

// Formatters
function formatTokens(num) {
  if (num === null || num === undefined) return '--';
  if (num >= 1_000_000) {
    return (num / 1_000_000).toFixed(2) + 'M';
  } else if (num >= 1_000) {
    return (num / 1_000).toFixed(1) + 'K';
  }
  return Number(num).toLocaleString();
}

function formatExact(num) {
  if (num === null || num === undefined) return '0';
  return Number(num).toLocaleString();
}

function formatDate(isoStr) {
  if (!isoStr) return '--';
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return isoStr;
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  } catch {
    return isoStr;
  }
}

// Toast Notification Helper
function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  let icon = 'ℹ️';
  if (type === 'success') icon = '✅';
  else if (type === 'error') icon = '⚠️';
  toast.innerHTML = `<span>${icon}</span><span>${message}</span>`;
  container.appendChild(toast);
  requestAnimationFrame(() => {
    toast.classList.add('show');
  });
  setTimeout(() => {
    toast.classList.remove('show');
    setTimeout(() => {
      if (toast.parentNode) toast.parentNode.removeChild(toast);
    }, 300);
  }, 3200);
}

// App Initialization
document.addEventListener('DOMContentLoaded', () => {
  initEventListeners();
  loadAllData();
  setupAutoRefresh();
});

function initEventListeners() {
  // Sync Button
  document.getElementById('btnSync').addEventListener('click', async () => {
    const btn = document.getElementById('btnSync');
    btn.disabled = true;
    btn.innerHTML = `<svg class="spin" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg> 同步中...`;
    const t0 = performance.now();
    try {
      await fetch('/api/scan', { method: 'POST' });
      await loadAllData();
      const elapsed = Math.round(performance.now() - t0);
      showToast(`遥测数据同步完成 (耗时 ${elapsed}ms)`, 'success');
    } catch (e) {
      console.error(e);
      showToast(`同步失败: ${e.message}`, 'error');
    } finally {
      btn.disabled = false;
      btn.innerHTML = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16"/></svg> 立即同步`;
    }
  });

  // Export Button
  document.getElementById('btnExport').addEventListener('click', () => {
    const choice = confirm("点击 [确定] 导出当前视图的 Excel/CSV 报表\n点击 [取消] 导出完整 JSON 数据");
    const query = `agent=${currentAgent}&model=${currentModel}&date=${currentDate}`;
    showToast('正在导出报表文件...', 'info');
    if (choice) {
      window.location.href = `/api/export?format=csv&${query}`;
    } else {
      window.location.href = `/api/export?format=json&${query}`;
    }
  });

  // Agent Selector Tabs
  document.querySelectorAll('.agent-tab').forEach(tab => {
    tab.addEventListener('click', (e) => {
      document.querySelectorAll('.agent-tab').forEach(t => t.classList.remove('active'));
      const target = e.currentTarget;
      target.classList.add('active');
      currentAgent = target.dataset.agent;
      currentPage = 0;
      loadAllData();
    });
  });

  // Date Filter Select
  document.getElementById('dateFilterSelect').addEventListener('change', (e) => {
    const val = e.target.value;
    if (val === 'today') {
      currentDate = getLocalDateString();
    } else {
      currentDate = val;
    }
    currentPage = 0;
    loadAllData();
  });

  // Model Filter Select
  document.getElementById('modelFilterSelect').addEventListener('change', (e) => {
    currentModel = e.target.value;
    currentPage = 0;
    loadAllData();
  });

  // Auto refresh select
  document.getElementById('autoRefreshSelect').addEventListener('change', setupAutoRefresh);

  // Trend Granularity Tabs
  document.querySelectorAll('#trendGranularityTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      document.querySelectorAll('#trendGranularityTabs .tab-btn').forEach(b => b.classList.remove('active'));
      e.target.classList.add('active');
      currentGranularity = e.target.dataset.granularity;
      loadTrends();
    });
  });

  // Search input with debounce
  let searchTimer;
  document.getElementById('searchInput').addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      searchQuery = e.target.value.trim();
      currentPage = 0;
      loadConversations();
    }, 300);
  });

  // Sort Select
  document.getElementById('sortSelect').addEventListener('change', (e) => {
    currentSort = e.target.value;
    currentPage = 0;
    loadConversations();
  });

  // Pagination
  document.getElementById('btnPrevPage').addEventListener('click', () => {
    if (currentPage > 0) {
      currentPage--;
      loadConversations();
    }
  });

  document.getElementById('btnNextPage').addEventListener('click', () => {
    currentPage++;
    loadConversations();
  });

  // Drawer Close
  document.getElementById('btnDrawerClose').addEventListener('click', closeDrawer);
  document.getElementById('drawerBackdrop').addEventListener('click', closeDrawer);

  // Clear Model Filter Button
  const btnClearModel = document.getElementById('btnClearModelFilter');
  if (btnClearModel) {
    btnClearModel.addEventListener('click', () => {
      currentModel = 'all';
      const sel = document.getElementById('modelFilterSelect');
      if (sel) sel.value = 'all';
      currentPage = 0;
      loadAllData();
    });
  }
}

function setupAutoRefresh() {
  if (autoRefreshTimer) clearInterval(autoRefreshTimer);
  const seconds = parseInt(document.getElementById('autoRefreshSelect').value, 10);
  if (seconds > 0) {
    autoRefreshTimer = setInterval(() => {
      loadAllData(true);
    }, seconds * 1000);
    document.getElementById('liveStatusText').textContent = `实时同步中 (${seconds}s)`;
  } else {
    document.getElementById('liveStatusText').textContent = '自动刷新已暂停';
  }
}

async function loadAllData(silent = false) {
  try {
    if (currentDate === 'today') {
      currentDate = getLocalDateString();
    }
    await Promise.all([
      loadSummary(),
      loadDailyRecords(),
      loadTrends(),
      loadConversations(),
      loadAgentNavPills(),
      loadModelsList()
    ]);
  } catch (err) {
    console.error("Failed to load dashboard data:", err);
  }
}

// 1. Load Summary
async function loadSummary() {
  const url = `/api/summary?agent=${currentAgent}&model=${currentModel}&date=${currentDate}`;
  const res = await fetch(url);
  const data = await res.json();
  latestSummaryData = data;

  // KPI 1: Total Tokens
  document.getElementById('valTotalTokens').textContent = formatTokens(data.total_tokens);
  document.getElementById('valTotalTokensRaw').textContent = formatExact(data.total_tokens);
  document.getElementById('kpiTodayTotalBadge').textContent = `+${formatTokens(data.today_total)} 今日`;

  // KPI 2: Input Tokens
  document.getElementById('valInputTokens').textContent = formatTokens(data.total_input);
  const inRatio = data.total_tokens > 0 ? ((data.total_input / data.total_tokens) * 100).toFixed(1) : 0;
  document.getElementById('valInputRatio').textContent = `${inRatio}% 占比`;
  document.getElementById('valTodayInput').textContent = formatTokens(data.today_input);

  // KPI 3: Output Tokens
  document.getElementById('valOutputTokens').textContent = formatTokens(data.total_output);
  const outRatio = data.total_tokens > 0 ? ((data.total_output / data.total_tokens) * 100).toFixed(1) : 0;
  document.getElementById('valOutputRatio').textContent = `${outRatio}% 占比`;
  document.getElementById('valTodayOutput').textContent = formatTokens(data.today_output);

  // KPI 4: Cache Read
  document.getElementById('valCacheTokens').textContent = formatTokens(data.total_cache);
  document.getElementById('valCacheHitRate').textContent = `${data.cache_hit_ratio}% 命中率`;
  document.getElementById('valCacheRatioDesc').textContent = `${formatTokens(data.total_cache)} tokens`;

  // KPI 5: Active Models
  const models = data.models_breakdown || [];
  if (currentModel !== 'all') {
    document.getElementById('valModelsCount').textContent = '已筛选该模型';
    document.getElementById('valPrimaryModel').textContent = currentModel;
    document.getElementById('valPrimaryModelTokens').textContent = formatTokens(data.total_tokens);
  } else {
    document.getElementById('valModelsCount').textContent = `${models.length} 种模型`;
    if (models.length > 0) {
      document.getElementById('valPrimaryModel').textContent = models[0].model_display_name;
      document.getElementById('valPrimaryModelTokens').textContent = formatTokens(models[0].total_tokens);
    } else {
      document.getElementById('valPrimaryModel').textContent = '--';
      document.getElementById('valPrimaryModelTokens').textContent = '--';
    }
  }

  // KPI 6: Conversations & Steps
  const agentLabel = currentAgent === 'all' ? '全平台' : (AGENT_CONFIG[currentAgent]?.name || currentAgent);
  document.getElementById('valAgentTypeBadge').textContent = agentLabel;
  document.getElementById('valConversations').textContent = `${data.total_convs} 个会话`;
  document.getElementById('valTotalSteps').textContent = formatExact(data.total_steps);
  document.getElementById('valAvgStepTokens').textContent = formatTokens(data.avg_tokens_per_step);
}

// 始终获取全平台各 Agent 的 Token 总量，切换 Agent 卡片时不随之归零或变动
async function loadAgentNavPills() {
  try {
    const url = `/api/summary?agent=all&model=all&date=${currentDate}`;
    const res = await fetch(url);
    const data = await res.json();
    renderAgentNavPills(data.agents_breakdown || [], data.total_tokens);
  } catch (err) {
    console.error("Failed to load agent nav pills:", err);
  }
}

function renderAgentNavPills(agents, totalTokens) {
  const map = {
    codex: 0,
    claude: 0,
    pi: 0,
    qoder: 0,
    antigravity: 0,
    workbuddy: 0,
    zcode: 0
  };
  let computedTotal = 0;
  agents.forEach(a => {
    map[a.agent_type] = a.total_tokens || 0;
    computedTotal += a.total_tokens || 0;
  });

  const finalTotal = (totalTokens !== undefined && totalTokens !== null) ? totalTokens : computedTotal;

  const setPill = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.textContent = formatTokens(val || 0);
  };

  setPill('pillTotalAll', finalTotal);
  setPill('pillTotalCodex', map['codex']);
  setPill('pillTotalClaude', map['claude']);
  setPill('pillTotalPi', map['pi']);
  setPill('pillTotalQoder', map['qoder']);
  setPill('pillTotalAg', map['antigravity']);
  setPill('pillTotalWb', map['workbuddy']);
  setPill('pillTotalZcode', map['zcode']);
}

// 2. Load Daily Records Timeline (按照天数区分记录，保持日历稳定展示)
async function loadDailyRecords() {
  const url = `/api/daily?agent=all&model=all&limit=60`;
  const res = await fetch(url);
  const dailyList = await res.json();

  const container = document.getElementById('dailyCardsContainer');
  container.innerHTML = '';

  const todayStr = getLocalDateString();
  const hasToday = dailyList && dailyList.some(d => d.date_bucket === todayStr || d.is_today);
  if (!hasToday && dailyList) {
    dailyList.unshift({
      date_bucket: todayStr,
      is_today: true,
      conv_count: 0,
      total_tokens: 0,
      total_input: 0,
      total_output: 0,
      cache_tokens: 0,
      total_steps: 0,
      agents_list: [],
      models_list: []
    });
  }

  // Update date selector dropdown
  updateDateSelectOptions(dailyList || []);

  if (!dailyList || dailyList.length === 0) {
    container.innerHTML = '<div style="color: var(--text-dim); padding: 20px;">暂无该条件下的天数记录</div>';
    return;
  }

  dailyList.forEach(d => {
    const card = document.createElement('div');
    const isActive = currentDate === d.date_bucket || (currentDate === 'today' && d.is_today);
    card.className = `daily-card ${isActive ? 'active' : ''}`;
    card.title = `点击切换筛选：仅看 ${d.date_bucket} 的所有会话与指标`;
    card.onclick = () => {
      if (currentDate === d.date_bucket || (currentDate === 'today' && d.is_today)) {
        currentDate = 'all'; // toggle off
      } else {
        currentDate = d.date_bucket;
      }
      document.getElementById('dateFilterSelect').value = currentDate;
      currentPage = 0;
      loadAllData();
    };

    let tagsHtml = '';
    (d.agents_list || []).forEach(ag => {
      const conf = AGENT_CONFIG[ag] || { name: ag, icon: '🤖', badgeClass: 'badge-project' };
      tagsHtml += `<span class="badge ${conf.badgeClass}" style="font-size: 0.68rem; padding: 1px 5px;">${conf.icon} ${conf.name}</span>`;
    });
    (d.models_list || []).slice(0, 2).forEach(m => {
      tagsHtml += `<span class="badge badge-model" style="font-size: 0.68rem; padding: 1px 5px;">${m}</span>`;
    });

    const todayTag = d.is_today ? '<span style="color: #10b981; font-size: 0.75rem; font-weight: bold;">[今日]</span>' : '';

    card.innerHTML = `
      <div class="daily-card-header">
        <div class="daily-card-date">
          <span>📅 ${d.date_bucket}</span>
          ${todayTag}
        </div>
        <div class="daily-card-tokens">${formatTokens(d.total_tokens)}</div>
      </div>
      <div class="daily-card-meta">
        <span>${d.conv_count} 个会话 · ${d.total_steps || 0} 轮交互</span>
        <span>缓存: ${formatTokens(d.cache_tokens)}</span>
      </div>
      <div class="daily-card-tags">
        ${tagsHtml}
      </div>
    `;
    container.appendChild(card);
  });
}

function updateDateSelectOptions(dailyList) {
  const sel = document.getElementById('dateFilterSelect');
  const todayStr = getLocalDateString();
  
  let opts = '';
  // 1. Today option (Default)
  const todayItem = dailyList.find(d => d.is_today || d.date_bucket === todayStr);
  const todayTokens = todayItem ? formatTokens(todayItem.total_tokens) : '--';
  const isTodaySelected = (currentDate === todayStr || currentDate === 'today');
  opts += `<option value="${todayStr}" ${isTodaySelected ? 'selected' : ''}>📅 今日 (${todayStr}) (${todayTokens}) [默认]</option>`;
  
  // 2. All time option
  const isAllSelected = (currentDate === 'all');
  opts += `<option value="all" ${isAllSelected ? 'selected' : ''}>🌐 全部历史天数 (All Time)</option>`;
  
  // 3. Other historical days
  dailyList.forEach(d => {
    if (d.date_bucket === todayStr) return; // already added above
    const isSelected = (d.date_bucket === currentDate) ? 'selected' : '';
    opts += `<option value="${d.date_bucket}" ${isSelected}>📅 ${d.date_bucket} (${formatTokens(d.total_tokens)})</option>`;
  });
  sel.innerHTML = opts;
}

// 始终获取当前 Agent 与当前日期范围下的所有模型列表 (model=all)，确保排行榜不被单一模型过滤缩小
async function loadModelsList() {
  try {
    const url = `/api/summary?agent=${currentAgent}&model=all&date=${currentDate}`;
    const res = await fetch(url);
    const data = await res.json();
    const models = data.models_breakdown || [];
    renderModelsList(models);
    updateModelSelectOptions(models);
  } catch (err) {
    console.error("Failed to load models list:", err);
  }
}

function renderModelsList(models) {
  const listEl = document.getElementById('modelsList');
  const countEl = document.getElementById('modelCountBadge');
  const btnClear = document.getElementById('btnClearModelFilter');

  if (btnClear) {
    if (currentModel !== 'all') {
      btnClear.style.display = 'inline-flex';
      btnClear.textContent = `✕ 返回全部 (已筛选: ${currentModel})`;
    } else {
      btnClear.style.display = 'none';
    }
  }

  if (countEl) {
    countEl.textContent = `共 ${models.length} 种在用模型`;
  }
  listEl.innerHTML = '';

  if (!models || models.length === 0) {
    listEl.innerHTML = '<div style="color: var(--text-dim); padding: 15px; text-align: center;">暂无模型数据</div>';
    return;
  }

  const maxTokens = Math.max(...models.map(m => m.total_tokens), 1);

  models.forEach((m, idx) => {
    const isSelected = (currentModel === m.model_display_name);
    const percent = ((m.total_tokens / maxTokens) * 100).toFixed(1);
    const item = document.createElement('div');
    item.className = `project-item ${isSelected ? 'active' : ''}`;
    item.style.cursor = 'pointer';
    item.title = isSelected
      ? `当前已筛选 ${m.model_display_name}，再次点击取消筛选并返回全部模型`
      : `点击筛选仅看 ${m.model_display_name} 会话与指标`;

    item.onclick = () => {
      if (currentModel === m.model_display_name) {
        // Toggle off: return to all models
        currentModel = 'all';
      } else {
        // Filter by model
        currentModel = m.model_display_name;
      }
      const sel = document.getElementById('modelFilterSelect');
      if (sel) sel.value = currentModel;
      currentPage = 0;
      loadAllData();
    };

    let rankClass = '';
    if (idx === 0) rankClass = 'rank-1';
    else if (idx === 1) rankClass = 'rank-2';
    else if (idx === 2) rankClass = 'rank-3';

    const rankBadge = `<span class="model-rank-badge ${rankClass}">#${idx + 1}</span>`;

    const activePill = isSelected 
      ? '<span class="model-active-pill">✓ 已筛选 (点击取消)</span>' 
      : '';

    item.innerHTML = `
      <div class="project-item-header">
        <span style="font-weight: 600; color: #fbbf24; display: flex; align-items: center; gap: 4px;">
          ${rankBadge} 🤖 ${m.model_display_name} ${activePill}
        </span>
        <span style="font-weight: 700; color: #fff;">${formatTokens(m.total_tokens)}</span>
      </div>
      <div class="project-progress-bar">
        <div class="project-progress-fill" style="width: ${percent}%; background: linear-gradient(90deg, #f59e0b, #ec4899);"></div>
      </div>
      <div class="project-stats">
        <span>${m.conv_count} 个会话</span>
        <span>入: ${formatTokens(m.input_tokens)} | 出: ${formatTokens(m.output_tokens)} | 缓: ${formatTokens(m.cache_tokens)}</span>
      </div>
    `;
    listEl.appendChild(item);
  });
}

let allCachedModels = [];

async function updateModelSelectOptions(currentScopeModels) {
  const sel = document.getElementById('modelFilterSelect');
  if (!sel) return;
  const currentVal = currentModel;

  if (!allCachedModels || allCachedModels.length === 0) {
    try {
      const res = await fetch('/api/models');
      allCachedModels = await res.json();
    } catch (e) {
      console.error(e);
      allCachedModels = currentScopeModels || [];
    }
  }

  const modelsToUse = (currentScopeModels && currentScopeModels.length > 0) ? currentScopeModels : allCachedModels;

  let opts = `<option value="all" ${currentVal === 'all' ? 'selected' : ''}>🎯 全部模型型号 (All Models)</option>`;
  let foundCurrent = (currentVal === 'all');

  modelsToUse.forEach(m => {
    const name = m.model_display_name;
    if (name === currentVal) foundCurrent = true;
    const isSelected = (name === currentVal) ? 'selected' : '';
    opts += `<option value="${name}" ${isSelected}>🤖 ${name} (${formatTokens(m.total_tokens)})</option>`;
  });

  if (!foundCurrent && currentVal !== 'all') {
    opts += `<option value="${currentVal}" selected>🤖 ${currentVal}</option>`;
  }

  sel.innerHTML = opts;
}

// 3. Load Trends Chart
async function loadTrends() {
  const url = `/api/trends?granularity=${currentGranularity}&days=30&agent=${currentAgent}&model=${currentModel}&date=${currentDate}`;
  const res = await fetch(url);
  const trends = await res.json();

  const labels = trends.map(t => {
    if (currentGranularity === 'hour') return t.time_bucket.slice(5);
    return t.time_bucket.slice(5);
  });

  const inputData = trends.map(t => t.input_tokens);
  const outputData = trends.map(t => t.output_tokens);
  const cacheData = trends.map(t => t.cache_tokens);

  const ctx = document.getElementById('trendChart').getContext('2d');
  if (trendChartInstance) {
    trendChartInstance.destroy();
  }

  trendChartInstance = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: labels,
      datasets: [
        {
          label: '输入 Tokens',
          data: inputData,
          backgroundColor: 'rgba(59, 130, 246, 0.75)',
          hoverBackgroundColor: '#3b82f6',
          borderColor: '#3b82f6',
          borderWidth: 1,
          borderRadius: 4,
          maxBarThickness: 32,
          stack: 'tokens'
        },
        {
          label: '输出 Tokens',
          data: outputData,
          backgroundColor: 'rgba(236, 72, 153, 0.85)',
          hoverBackgroundColor: '#ec4899',
          borderColor: '#ec4899',
          borderWidth: 1,
          borderRadius: 4,
          maxBarThickness: 32,
          stack: 'tokens'
        },
        {
          label: 'Prompt 缓存读取 (Cache)',
          data: cacheData,
          type: 'line',
          borderColor: '#10b981',
          backgroundColor: 'rgba(16, 185, 129, 0.12)',
          borderWidth: 2.5,
          pointRadius: 3,
          pointHoverRadius: 6,
          pointBackgroundColor: '#10b981',
          fill: true,
          tension: 0.4
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: {
            color: '#94a3b8',
            font: { size: 11, family: 'Inter, system-ui, sans-serif' },
            usePointStyle: true,
            boxWidth: 8
          }
        },
        tooltip: {
          mode: 'index',
          intersect: false,
          backgroundColor: 'rgba(15, 23, 42, 0.95)',
          borderColor: 'rgba(255, 255, 255, 0.12)',
          borderWidth: 1,
          padding: 10,
          titleColor: '#f8fafc',
          bodyColor: '#cbd5e1',
          cornerRadius: 8,
          callbacks: {
            label: (ctx) => `  ${ctx.dataset.label}: ${formatTokens(ctx.raw)} (${formatExact(ctx.raw)})`
          }
        }
      },
      scales: {
        x: {
          stacked: true,
          grid: { color: 'rgba(255, 255, 255, 0.04)' },
          ticks: { color: '#64748b', font: { size: 11 } }
        },
        y: {
          grid: { color: 'rgba(255, 255, 255, 0.04)' },
          ticks: { color: '#64748b', font: { size: 11 }, callback: (v) => formatTokens(v) }
        }
      }
    }
  });
}

// 4. Load Conversations Table
async function loadConversations() {
  const offset = currentPage * pageSize;
  const url = `/api/conversations?agent=${currentAgent}&model=${currentModel}&date=${currentDate}&sort=${currentSort}&order=desc&search=${encodeURIComponent(searchQuery)}&limit=${pageSize}&offset=${offset}`;
  
  const res = await fetch(url);
  const data = await res.json();

  const tbody = document.getElementById('conversationTableBody');
  tbody.innerHTML = '';

  if (!data.items || data.items.length === 0) {
    let emptyHtml = `
      <div style="padding: 30px 15px; text-align: center;">
        <div style="font-size: 1rem; color: #cbd5e1; margin-bottom: 8px;">
          ${currentDate !== 'all' ? `📅 所选日期【${currentDate}】暂无匹配会话` : '暂无匹配的会话数据'}
        </div>
        <div style="font-size: 0.85rem; color: var(--text-dim); margin-bottom: 14px;">
          ${currentDate !== 'all' ? '系统默认进入「今日」视图。若您在寻找其他历史会话（如 WorkBuddy 或历史 Qoder 会话），可一键切换为全部天数。' : '请调整筛选条件或搜索关键词重试。'}
        </div>
        ${currentDate !== 'all' ? `
          <button class="btn btn-primary" style="padding: 6px 16px; font-size: 0.84rem;" onclick="switchToAllDates()">
            🌐 查看全部历史天数会话 (All Time)
          </button>
        ` : ''}
      </div>
    `;
    tbody.innerHTML = `<tr><td colspan="10">${emptyHtml}</td></tr>`;
    updatePagination(0, 0);
    return;
  }

  const maxTokens = Math.max(...data.items.map(c => c.total_tokens), 1);

  data.items.forEach(c => {
    const tr = document.createElement('tr');

    const agentConf = AGENT_CONFIG[c.agent_type] || { name: c.agent_type, icon: '🤖', badgeClass: 'badge-project' };
    const agentBadge = `<span class="badge ${agentConf.badgeClass}">${agentConf.icon} ${agentConf.name}</span>`;
    const modelList = Array.from(new Set((c.model_display_name || 'Auto').split(',').map(m => m.trim()))).filter(Boolean);
    const modelBadge = modelList.map(m => `<span class="badge badge-model" style="margin-right: 3px; margin-bottom: 2px;">${m}</span>`).join('');

    const barWidth = Math.max(Math.min((c.total_tokens / maxTokens) * 100, 100), 2);
    const titleText = c.title || c.preview || '无标题会话';
    const projectLabel = c.project_id === 'outside-of-project' ? '独立' : (c.project_id ? c.project_id.slice(0, 12) : '--');
    const dateLabel = c.date_bucket ? `<span style="font-family: monospace; font-size: 0.78rem; color: #a5b4fc;">${c.date_bucket}</span>` : '--';

    tr.innerHTML = `
      <td>${dateLabel}</td>
      <td>${agentBadge}</td>
      <td>${modelBadge}</td>
      <td class="conv-title-cell">
        <div class="conv-title-text" title="${titleText}" onclick="openConversationDetail('${c.conversation_id}')">
          ${titleText}
        </div>
        <div class="conv-meta-badges">
          <span style="font-family: monospace; font-size: 0.72rem; color: var(--text-dim); cursor: pointer;" title="点击复制ID" onclick="copyId('${c.conversation_id}')">
            #${c.conversation_id.slice(0, 8)}
          </span>
        </div>
      </td>
      <td><span class="badge badge-project" title="${c.project_id}">${projectLabel}</span></td>
      <td style="font-weight: 600;">${c.step_records_count || c.step_count} 轮</td>
      <td class="token-cell" style="color: #10b981;">${formatTokens(c.total_cache_tokens)}</td>
      <td class="token-cell">
        <div>入: ${formatTokens(c.total_input_tokens)}</div>
        <div class="token-split">出: ${formatTokens(c.total_output_tokens)}</div>
      </td>
      <td class="token-cell">
        <div class="token-total">${formatTokens(c.total_tokens)}</div>
        <div class="project-progress-bar" style="margin-top: 4px; height: 4px;">
          <div class="project-progress-fill" style="width: ${barWidth}%;"></div>
        </div>
      </td>
      <td style="text-align: right;">
        <button class="btn" style="padding: 4px 10px; font-size: 0.78rem;" onclick="openConversationDetail('${c.conversation_id}')">
          详情 &gt;
        </button>
      </td>
    `;
    tbody.appendChild(tr);
  });

  updatePagination(data.total, offset);
}

function updatePagination(total, offset) {
  const totalPages = Math.ceil(total / pageSize) || 1;
  document.getElementById('pageInfo').textContent = `第 ${currentPage + 1} / ${totalPages} 页 (共 ${total} 条)`;
  document.getElementById('btnPrevPage').disabled = currentPage <= 0;
  document.getElementById('btnNextPage').disabled = (offset + pageSize) >= total;
}

// 5. Open Single Conversation Drawer
async function openConversationDetail(cid) {
  const drawer = document.getElementById('conversationDrawer');
  const backdrop = document.getElementById('drawerBackdrop');
  
  drawer.classList.add('open');
  backdrop.classList.add('open');

  document.getElementById('drawerTitle').textContent = '加载中...';
  document.getElementById('drawerCid').textContent = cid;
  document.getElementById('drawerStepsBody').innerHTML = '<tr><td colspan="7" style="text-align:center; padding: 20px;">数据加载中...</td></tr>';

  try {
    const res = await fetch(`/api/conversation/${cid}`);
    const data = await res.json();

    const agentConf = AGENT_CONFIG[data.agent_type] || { name: data.agent_type, icon: '🤖', badgeClass: 'badge-project' };
    const agentEl = document.getElementById('drawerAgentBadge');
    agentEl.className = `badge ${agentConf.badgeClass}`;
    agentEl.textContent = `${agentConf.icon} ${agentConf.name}`;

    const modelEl = document.getElementById('drawerModelBadge');
    const modelSet = Array.from(new Set((data.model_display_name || 'Auto').split(',').map(m => m.trim()))).filter(Boolean);
    modelEl.textContent = `🤖 ${modelSet.join(' · ')}`;

    const dateEl = document.getElementById('drawerDateBadge');
    dateEl.textContent = `📅 ${data.date_bucket || '--'}`;

    document.getElementById('drawerTitle').textContent = data.title || '无标题会话';
    document.getElementById('drawerTotalTokens').textContent = formatTokens(data.total_tokens);
    
    const cacheTotal = (data.total_input_tokens + data.total_cache_tokens);
    const cacheRate = cacheTotal > 0 ? ((data.total_cache_tokens / cacheTotal) * 100).toFixed(1) : 0;
    document.getElementById('drawerCacheRate').textContent = `${cacheRate}%`;
    document.getElementById('drawerStepCount').textContent = `${data.steps ? data.steps.length : (data.step_count || 1)} 轮`;

    renderStepGrowthChart(data.steps || []);

    // Subagents
    const subSection = document.getElementById('drawerSubagentsSection');
    const subList = document.getElementById('drawerSubagentsList');
    if (data.subagents && data.subagents.length > 0) {
      subSection.style.display = 'block';
      subList.innerHTML = '';
      data.subagents.forEach(s => {
        const item = document.createElement('div');
        item.className = 'project-item';
        item.innerHTML = `
          <div class="project-item-header">
            <span>🤖 ${s.agent_role || 'Subagent'} [${s.model_display_name}]</span>
            <span style="color: #fff;">${formatTokens(s.total_tokens)}</span>
          </div>
          <div class="project-stats">
            <span>${s.step_records_count} 轮交互</span>
            <span style="color: var(--primary-light); cursor: pointer;" onclick="openConversationDetail('${s.conversation_id}')">查看详情 &gt;</span>
          </div>
        `;
        subList.appendChild(item);
      });
    } else {
      subSection.style.display = 'none';
    }

    // Steps Table
    const stepsBody = document.getElementById('drawerStepsBody');
    stepsBody.innerHTML = '';

    if (!data.steps || data.steps.length === 0) {
      stepsBody.innerHTML = '<tr><td colspan="7" style="text-align:center; color: var(--text-dim); padding: 15px;">该会话仅有总体汇总，无单步流水</td></tr>';
      return;
    }

    data.steps.forEach(s => {
      const tr = document.createElement('tr');
      let toolsHtml = '<span style="color: var(--text-dim);">-</span>';
      if (s.tool_names) {
        toolsHtml = s.tool_names.split(',').map(name => `<span class="tool-tag">${name}</span>`).join('');
      }

      tr.innerHTML = `
        <td style="font-weight: 600;">#${s.step_index}</td>
        <td><span class="badge badge-model" style="font-size: 0.7rem;">${s.model_display_name || '--'}</span></td>
        <td style="font-family: monospace;">${formatTokens(s.input_tokens)}</td>
        <td style="font-family: monospace; color: #10b981;">${formatTokens(s.cache_read_tokens)}</td>
        <td style="font-family: monospace; color: #ec4899;">${formatTokens(s.output_tokens)}</td>
        <td>${toolsHtml}</td>
        <td style="font-size: 0.72rem; color: var(--text-dim);">${formatDate(s.created_at)}</td>
      `;
      stepsBody.appendChild(tr);
    });

  } catch (err) {
    console.error(err);
    document.getElementById('drawerTitle').textContent = '加载失败';
  }
}

function renderStepGrowthChart(steps) {
  const ctx = document.getElementById('stepGrowthChart').getContext('2d');
  if (stepGrowthChartInstance) {
    stepGrowthChartInstance.destroy();
  }

  const labels = steps.map(s => `#${s.step_index}`);
  const inputData = steps.map(s => s.input_tokens);
  const cacheData = steps.map(s => s.cache_read_tokens);
  const outputData = steps.map(s => s.output_tokens);

  stepGrowthChartInstance = new Chart(ctx, {
    type: 'line',
    data: {
      labels: labels,
      datasets: [
        {
          label: '输入上下文 (Input Context)',
          data: inputData,
          borderColor: '#6366f1',
          backgroundColor: 'rgba(99, 102, 241, 0.12)',
          fill: true,
          tension: 0.4,
          borderWidth: 2,
          pointRadius: 3,
          pointHoverRadius: 5
        },
        {
          label: '缓存读取 (Cache Hit)',
          data: cacheData,
          borderColor: '#10b981',
          borderWidth: 2,
          pointRadius: 2,
          pointHoverRadius: 4,
          borderDash: [4, 4]
        },
        {
          label: '模型生成 (Output Tokens)',
          data: outputData,
          borderColor: '#ec4899',
          borderWidth: 2,
          pointRadius: 2,
          pointHoverRadius: 4
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: {
            color: '#94a3b8',
            font: { size: 10, family: 'Inter, system-ui, sans-serif' },
            usePointStyle: true,
            boxWidth: 8
          }
        },
        tooltip: {
          mode: 'index',
          intersect: false,
          backgroundColor: 'rgba(15, 23, 42, 0.95)',
          borderColor: 'rgba(255, 255, 255, 0.12)',
          borderWidth: 1,
          padding: 8,
          titleColor: '#f8fafc',
          bodyColor: '#cbd5e1',
          cornerRadius: 6,
          callbacks: {
            label: (ctx) => `  ${ctx.dataset.label}: ${formatTokens(ctx.raw)} (${formatExact(ctx.raw)})`
          }
        }
      },
      scales: {
        x: { grid: { color: 'rgba(255, 255, 255, 0.04)' }, ticks: { color: '#64748b' } },
        y: { 
          grid: { color: 'rgba(255, 255, 255, 0.04)' }, 
          ticks: { color: '#64748b', callback: (v) => formatTokens(v) } 
        }
      }
    }
  });
}

function closeDrawer() {
  document.getElementById('conversationDrawer').classList.remove('open');
  document.getElementById('drawerBackdrop').classList.remove('open');
}

function copyId(id) {
  navigator.clipboard.writeText(id).then(() => {
    showToast(`已复制会话 ID: #${id.slice(0, 8)}`, 'success');
  }).catch(() => {
    prompt('会话ID:', id);
  });
}

function switchToAllDates() {
  currentDate = 'all';
  const sel = document.getElementById('dateFilterSelect');
  if (sel) sel.value = 'all';
  currentPage = 0;
  loadAllData();
}
